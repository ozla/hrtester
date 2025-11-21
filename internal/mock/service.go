package mock

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ozla/hrtester/internal/config"
	"github.com/ozla/hrtester/internal/log"
	"github.com/ozla/hrtester/internal/shared"
	"github.com/ozla/hrtester/internal/shared/middleware"
)

////////////////////////////////////////////////////////////////////////////////

const (
	statusReady uint32 = iota
	statusRunning
	statusStopping
)

////////////////////////////////////////////////////////////////////////////////

type params struct {
	Duration shared.Duration `json:"duration"`
	Response struct {
		HeaderLatency struct {
			Min shared.Duration `json:"min"`
			Max shared.Duration `json:"max"`
		} `json:"headerLatency"`
		Duration struct {
			Min shared.Duration `json:"min"`
			Max shared.Duration `json:"max"`
		} `json:"duration"`
	} `json:"response"`
}

////////////////////////////////////////////////////////////////////////////////

type service struct {
	server       *http.Server
	status       *atomic.Uint32
	terminated   chan struct{}
	shutdownOnce sync.Once
	params       params
	startedAt    time.Time
	runningUntil time.Time
}

func NewService() *service {
	s := &service{
		status:     &atomic.Uint32{},
		terminated: make(chan struct{}),
	}
	s.status.Store(statusReady)
	return s
}

func (s *service) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc(
		"/",
		middleware.WrapHandlerFuncs(
			s.handleDefault,
			middleware.DrainAndCloseHandler,
			middleware.DebugHandler,
		),
	)
	mux.HandleFunc(
		"/__service/",
		middleware.WrapHandlerFuncs(
			s.handleService,
			middleware.DrainAndCloseHandler,
		),
	)
	mux.HandleFunc(
		"/__mock",
		middleware.WrapHandlerFuncs(
			s.handleMock,
			middleware.DrainAndCloseHandler,
		),
	)

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Mocker.Port))
	if err != nil {
		log.Fatal("binding error", err, slog.Int("port", int(config.Mocker.Port)))
	}

	tlsEnabled := config.Mocker.Cert != "" && config.Mocker.Key != ""

	tlsStatus := "disabled"
	if tlsEnabled {
		tlsStatus = "enabled"
	}
	log.Info(
		"mock server is listening",
		slog.Int("port", int(config.Mocker.Port)),
		slog.String("tls", tlsStatus),
	)

	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: time.Minute,
	}

	go func() {
		if tlsEnabled {
			s.server.TLSConfig = &tls.Config{}
			cert, err := tls.LoadX509KeyPair(
				config.Mocker.Cert,
				config.Mocker.Key,
			)
			if err != nil {
				log.Fatal("error loading key pair", err)
			}
			s.server.TLSConfig.Certificates = []tls.Certificate{cert}
			if config.Mocker.CAs != "" {
				s.server.TLSConfig.ClientAuth = tls.RequireAndVerifyClientCert
				if pool, err := shared.CACertPool(config.Mocker.CAs); err != nil {
					log.Fatal("error loading trusted certificates", err)
				} else {
					s.server.TLSConfig.ClientCAs = pool
				}
			}
			l = tls.NewListener(l, s.server.TLSConfig)
		}
		err = s.server.Serve(l)
		if err != nil && err != http.ErrServerClosed {
			log.Fatal("server encountered an unexpected error", err)
		}
	}()

	<-s.terminated
}

func (s *service) shutdown() {
	s.shutdownOnce.Do(
		func() {
			log.Info("shutting down mock server; hrtester process will terminate")
			go func() {
				defer close(s.terminated)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := s.server.Shutdown(ctx); err != nil {
					log.Warn("server forced to shut down", slog.Any("err", err))
				}
			}()
		},
	)
}

////////////////////////////////////////////////////////////////////////////////

func (s *service) handleDefault(w http.ResponseWriter, r *http.Request) {
	if s.status.Load() != statusRunning {
		http.Error(w, "Service has not started.", http.StatusServiceUnavailable)
		return
	}

	var respDelay, headDelay time.Duration

	d := int64(s.params.Response.Duration.Max - s.params.Response.Duration.Min)
	respDelay = time.Duration(s.params.Response.Duration.Min)
	if d > 0 {
		respDelay += time.Duration(rand.Int64N(d))
	}

	d = int64(s.params.Response.HeaderLatency.Max - s.params.Response.HeaderLatency.Min)
	headDelay = time.Duration(s.params.Response.HeaderLatency.Min)
	if d > 0 {
		headDelay += time.Duration(rand.Int64N(d))
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if headDelay > 0 {
		log.Debug(
			"applying header delay",
			slog.Any("duration", shared.Duration(headDelay)),
			slog.Group("request",
				slog.String("remoteAddr", r.RemoteAddr),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			),
		)
		time.Sleep(headDelay)
	}
	w.WriteHeader(http.StatusOK)

	respDelay -= headDelay
	if respDelay <= 0 {
		return
	}

	log.Debug(
		"applying response delay",
		slog.Any("duration", shared.Duration(respDelay)),
		slog.Group("request",
			slog.String("remoteAddr", r.RemoteAddr),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
		),
	)
	time.Sleep(respDelay)
	w.Write([]byte("\n"))
}

func (s *service) handleService(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/__service", "/__service/":
		switch r.Method {
		case http.MethodGet:
			var body struct {
				Status   string          `json:"status"`
				Duration shared.Duration `json:"duration,omitempty"`
			}

			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)

			switch s.status.Load() {
			case statusReady:
				body.Status = "ready"
			case statusRunning:
				body.Status = "running"
				body.Duration = shared.Duration(time.Until(s.runningUntil))
			case statusStopping:
				body.Status = "stopping"
			}

			b, err := json.Marshal(body)
			if err != nil {
				log.Debug("failed to marshal response body", slog.Any("err", err))
				http.Error(
					w,
					http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError,
				)
				return
			}
			w.Write(b)
		default:
			w.Header().Set("Allow", http.MethodGet)
			http.Error(
				w,
				http.StatusText(http.StatusMethodNotAllowed),
				http.StatusMethodNotAllowed,
			)
			return
		}
	case "/__service/terminate", "/__service/terminate/":
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusOK)
			s.shutdown()
		default:
			w.Header().Set("Allow", http.MethodPost)
			http.Error(
				w,
				http.StatusText(http.StatusMethodNotAllowed),
				http.StatusMethodNotAllowed,
			)
			return
		}
	default:
		http.Error(
			w,
			http.StatusText(http.StatusNotFound),
			http.StatusNotFound,
		)
		return
	}
}

func (s *service) handleMock(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if b, err := io.ReadAll(r.Body); err == nil {
			if err = json.Unmarshal(b, &s.params); err != nil {
				http.Error(
					w,
					fmt.Sprintf("Malformed JSON: %v", err),
					http.StatusBadRequest,
				)
				return
			}
		} else {
			http.Error(
				w,
				"Failed to read request body",
				http.StatusBadRequest,
			)
			log.Debug("failed to read request body", slog.Any("err", err))
			return
		}
		log.Info(
			"loaded mock service config",
			slog.Any("duration", s.params.Duration),
			slog.Group(
				"headerLatency",
				slog.Any("min", s.params.Response.HeaderLatency.Min),
				slog.Any("max", s.params.Response.HeaderLatency.Max),
			),
			slog.Group(
				"responseDuration",
				slog.Any("min", s.params.Response.Duration.Min),
				slog.Any("max", s.params.Response.Duration.Max),
			),
		)
		if s.params.Duration < 0 {
			http.Error(
				w,
				"Invalid service duration: must be >= 0",
				http.StatusBadRequest,
			)
			return
		}
		if s.params.Response.HeaderLatency.Min < 0 ||
			s.params.Response.HeaderLatency.Min > s.params.Response.HeaderLatency.Max {
			http.Error(
				w,
				"Invalid header latency: min must be >= 0 and <= max",
				http.StatusBadRequest,
			)
			return
		}
		if s.params.Response.Duration.Min < 0 ||
			s.params.Response.Duration.Min > s.params.Response.Duration.Max {
			http.Error(
				w,
				"Invalid response duration: min must be >= 0 and <= max",
				http.StatusBadRequest,
			)
			return
		}

		if !s.status.CompareAndSwap(statusReady, statusRunning) {
			http.Error(
				w,
				"Mock service is already running.",
				http.StatusServiceUnavailable,
			)
			return
		}
		s.startedAt = time.Now()
		s.runningUntil = s.startedAt.Add(time.Duration(s.params.Duration))
		go func() {
			time.Sleep(time.Duration(s.params.Duration))
			s.status.Store(statusReady)
			log.Info(
				"mock service has stopped",
				slog.Time("startedAt", s.startedAt),
			)
		}()
		w.WriteHeader(http.StatusOK)
		log.Info(
			"mock service has started",
			slog.Time("finishesAt", s.runningUntil),
		)
	default:
		w.Header().Set("Allow", http.MethodPost)
		http.Error(
			w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed,
		)
	}
}

////////////////////////////////////////////////////////////////////////////////
