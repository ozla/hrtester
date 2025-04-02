package tester

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/ozla/hrtester/internal/config"
	"github.com/ozla/hrtester/internal/log"
	"github.com/ozla/hrtester/internal/shared"
	"github.com/ozla/hrtester/internal/shared/middleware"
)

////////////////////////////////////////////////////////////////////////////////

const (
	statusReady uint32 = iota
	statusTesting
	statusStopping

	idsBufferSize     = 100
	resultsBufferSize = 20

	spinupFactor      = 4
	spinupMaxDuration = int64(10 * time.Second)
)

////////////////////////////////////////////////////////////////////////////////

type service struct {
	server       *http.Server
	clientCert   *tls.Certificate
	rootCAs      *x509.CertPool
	status       *atomic.Uint32
	terminated   chan struct{}
	shutdownOnce sync.Once
	params       params
	testCtx      context.Context
	testCancel   context.CancelFunc
	startedAt    time.Time
	runningUntil time.Time
	idGenDone    chan struct{}
	testersDone  chan struct{}
	ids          chan uuid.UUID
	results      chan shared.TestResult
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
	if config.Tester.CAs != "" {
		if pool, err := shared.CACertPool(config.Tester.CAs); err != nil {
			log.Fatal("error loading trusted certificates", err)
		} else {
			s.rootCAs = pool
		}
	}
	if config.Tester.Cert != "" && config.Tester.Key != "" {
		if cert, err := tls.LoadX509KeyPair(
			config.Tester.Cert,
			config.Tester.Key,
		); err != nil {
			log.Fatal("error loading key pair", err)
		} else {
			s.clientCert = &cert
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc(
		"/test",
		middleware.WrapHandlerFuncs(
			s.handleTest,
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

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Tester.Port))
	if err != nil {
		log.Fatal("failed to bind server to port", err, slog.Int("port", int(config.Tester.Port)))
	}

	log.Info(
		"tester service is listening",
		slog.Int("port", int(config.Tester.Port)),
	)

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", config.Tester.Port),
		Handler:           mux,
		ReadHeaderTimeout: time.Minute,
	}

	go func() {
		if err := s.server.Serve(l); err != nil && err != http.ErrServerClosed {
			log.Fatal("server encountered an unexpected error", err)
		}
	}()

	<-s.terminated
}

func (s *service) shutdown() {
	s.shutdownOnce.Do(
		func() {
			log.Info("shutting down teseter server, hrtester test process will terminate")
			go func() {
				defer close(s.terminated)
				if s.testCancel != nil {
					s.testCancel()
				}
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

func (s *service) handleTest(w http.ResponseWriter, r *http.Request) {
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
		if s.params.Duration < 0 {
			http.Error(
				w,
				"Invalid service duration: must be >= 0",
				http.StatusBadRequest,
			)
			return
		}
		if s.params.ReqSchema == "" {
			s.params.ReqSchema = "http"
		}
		if s.params.ReqVersion == [...]uint8{0, 0} {
			s.params.ReqVersion = [...]uint8{1, 1}
		}
		if s.params.ReqIDHeader == "" {
			s.params.ReqIDHeader = "X-Request-ID"
		}
		log.Info(
			"loaded test service config",
			slog.String("name", s.params.Name),
			slog.Any("duration", s.params.Duration),
			slog.Any("pace", s.params.Pace),
			slog.Uint64("parallelTesters", uint64(s.params.ParallelTesters)),
		)

		if !s.status.CompareAndSwap(statusReady, statusTesting) {
			http.Error(
				w,
				"Service is already running. Please try again later.",
				http.StatusServiceUnavailable,
			)
		}
		s.startedAt = time.Now()
		s.runningUntil = s.startedAt.Add(time.Duration(s.params.Duration))
		ctx, cancel := context.WithDeadline(context.Background(), s.runningUntil)
		s.testCtx, s.testCancel = ctx, cancel
		s.startSender()
		s.startIDGen()
		s.startTesters()
		go func() {
			<-s.testCtx.Done()
			<-s.testersDone
			close(s.results)
			s.status.Store(statusReady)
			log.Info(
				"tester service has stopped",
				slog.Time("startedAt", s.startedAt),
			)
		}()
		w.WriteHeader(http.StatusOK)
		log.Info(
			"tester service has started",
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
			case statusTesting:
				body.Status = "testing"
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

////////////////////////////////////////////////////////////////////////////////

func (s *service) startSender() {
	s.results = make(
		chan shared.TestResult,
		s.params.ParallelTesters*resultsBufferSize,
	)
	go func() {
		c := &http.Client{
			Timeout: 1 * time.Second,
		}
		u := (&url.URL{Scheme: "http", Host: config.Tester.Collector}).String()
		for res := range s.results {
			if len(s.results) > cap(s.results)/2 {
				log.Warn(
					"results buffer saturation",
					slog.Int("precentage", len(s.results)*100/cap(s.results)),
				)
			}
			req, err := http.NewRequest(
				http.MethodPost,
				u,
				strings.NewReader(res.URLValues().Encode()),
			)
			if err != nil {
				log.Error("failed to create request for collector", err)
				continue
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			c.Do(req)
		}
	}()
	log.Debug("result sender started")
}

func (s *service) startIDGen() {
	s.ids = make(chan uuid.UUID, idsBufferSize)
	s.idGenDone = make(chan struct{})
	go func() {
		defer close(s.ids)
		for {
			select {
			case s.ids <- uuid.New():
			case <-s.idGenDone:
				return
			}
		}
	}()
	log.Debug("id generator started")
}

func (s *service) startTesters() {
	s.testersDone = make(chan struct{})
	go runTesters(s)
	log.Debug("target testers started")
}

////////////////////////////////////////////////////////////////////////////////

func runTesters(s *service) {
	totalRequests := &atomic.Uint64{}
	targetDuration :=
		time.Duration(
			int64(math.Floor(6.0e4/float64(s.params.Pace))),
		) * time.Millisecond
	log.Debug(
		"starting testers",
		slog.Int("paralletTesters", int(s.params.ParallelTesters)),
		slog.String("targetDuration", targetDuration.Truncate(time.Millisecond).String()),
	)

	// Stagger tester startup across min(spinupMaxDuration, 1/spinupFactor of total test duration)
	spinup := time.Duration(s.params.Duration).Nanoseconds() / int64(spinupFactor)
	if spinup > spinupMaxDuration {
		spinup = spinupMaxDuration
	}
	spinup = spinup / int64(s.params.ParallelTesters)

	wg := sync.WaitGroup{}
	for i := range int(s.params.ParallelTesters) {
		time.Sleep(time.Duration(rand.Int64N(spinup)))
		wg.Add(1)
		go func() {
			defer wg.Done()

			log.Debug("starting tester", slog.Int("num", i))

			var (
				client         = &http.Client{}
				randSrc        = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
				targetDuration = targetDuration * time.Duration(s.params.ParallelTesters)
				localN         = 0

				r request
			)

			if s.params.ReqSchema == "https" {
				c := tls.Config{}
				if config.Tester.SkipNameCheck {
					c.InsecureSkipVerify = true
					c.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
						opts := x509.VerifyOptions{
							Roots: s.rootCAs,
						}
						var (
							cert *x509.Certificate
							err  error
						)
						switch n := len(rawCerts); {
						case n == 0:
							return fmt.Errorf("no server certificate received")
						case n == 1:
							if cert, err = x509.ParseCertificate(rawCerts[0]); err != nil {
								return fmt.Errorf("failed to parse certificate: %w", err)
							}
						default:
							if cert, err = x509.ParseCertificate(rawCerts[0]); err != nil {
								return fmt.Errorf("failed to parse certificate: %w", err)
							}
							opts.Intermediates = x509.NewCertPool()
							for _, rc := range rawCerts[1:] {
								c, err := x509.ParseCertificate(rc)
								if err != nil {
									return fmt.Errorf("failed to parse certificate: %w", err)
								}
								opts.Intermediates.AddCert(c)
							}
						}
						_, err = cert.Verify(opts)
						if err != nil {
							return fmt.Errorf("certificate verification failed: %w", err)
						}
						return nil
					}
				}
				if s.rootCAs != nil {
					c.RootCAs = s.rootCAs
				}
				if s.clientCert != nil {
					c.Certificates = []tls.Certificate{*s.clientCert}
				}
				client.Transport = &http.Transport{
					IdleConnTimeout:     30 * time.Second,
					MaxIdleConns:        100,
					MaxIdleConnsPerHost: int(s.params.ParallelTesters),
					TLSClientConfig:     &c,
				}
			} else {
				client.Transport = &http.Transport{
					IdleConnTimeout:     30 * time.Second,
					MaxIdleConns:        100,
					MaxIdleConnsPerHost: int(s.params.ParallelTesters),
				}
			}

			for {
				select {
				case <-s.testCtx.Done():
					return
				default:
					globalN := totalRequests.Add(1)
					localN++
					if n := len(s.params.Requests); n == 1 {
						r = s.params.Requests[0]
					} else {
						switch s.params.Choice {
						case "roundrobin":
							r = s.params.Requests[localN%n]
						case "random":
							r = s.params.Requests[randSrc.IntN(n)]
						}
					}

					u := &url.URL{
						Scheme: string(s.params.ReqSchema),
						Host:   config.Tester.Target,
						Path:   r.Path,
					}
					reqCtx, reqCancel := context.WithTimeout(
						context.Background(),
						time.Duration(s.params.Timeout),
					)
					req, err := http.NewRequestWithContext(
						reqCtx,
						string(r.Method),
						u.String(),
						strings.NewReader(r.Body),
					)
					if err != nil {
						log.Error("failed to create request", err)
						reqCancel()
						continue
					}
					for k, v := range r.Header {
						req.Header[k] = append([]string(nil), v...)
					}
					id := <-s.ids
					req.Header.Add(s.params.ReqIDHeader, id.String())

					var tRes shared.TestResult
					start := time.Now()
					log.Debug(
						"request",
						slog.Group(
							"client",
							slog.Int("num", i),
						),
						slog.Group(
							"request",
							slog.Int("num", int(globalN)),
							slog.String("path", r.Path),
						),
					)
					tRes.SetRequestTime(start.Truncate(time.Millisecond))
					resp, err := client.Do(req)
					reqCancel()
					if err != nil {
						if errors.Is(err, context.DeadlineExceeded) {
							tRes.SetTimedOut(true)
						} else {
							log.Error("request failed", err, slog.Any("url", u))
						}
					} else {
						tRes.SetTimedOut(false)
					}
					elapsed := time.Since(start).Truncate(time.Millisecond)
					tRes.SetTestName(s.params.Name)
					tRes.SetRequestID(id)
					tRes.SetRequestNum(globalN)
					tRes.SetRequesMethod(string(r.Method))
					tRes.SetRequestPath(r.Path)
					tRes.SetRoundDuration(shared.Duration(elapsed))
					if resp != nil {
						tRes.SetResponseCode(resp.StatusCode)
						io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}
					if elapsed < targetDuration {
						time.Sleep(targetDuration - elapsed)
					}
					s.results <- tRes
				}
			}
		}()
	}
	wg.Wait()
	close(s.testersDone)
}

////////////////////////////////////////////////////////////////////////////////
