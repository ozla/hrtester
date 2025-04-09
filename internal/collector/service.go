package collector

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/ozla/hrtester/internal/config"
	"github.com/ozla/hrtester/internal/log"
	"github.com/ozla/hrtester/internal/shared"
	"github.com/ozla/hrtester/internal/shared/middleware"
)

////////////////////////////////////////////////////////////////////////////////

const (
	BufferSize    = 10
	FlushInterval = 1000
)

////////////////////////////////////////////////////////////////////////////////

type service struct {
	server       *http.Server
	terminated   chan struct{}
	shutdownOnce sync.Once
	cancelWrite  context.CancelFunc
	results      chan shared.TestResult
	csv          io.WriteCloser
}

func NewCollectService() *service {
	s := &service{
		terminated: make(chan struct{}),
		results:    make(chan shared.TestResult, BufferSize),
	}
	return s
}

func (s *service) Start() {
	f, err := os.OpenFile(config.Collector.CSVFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal("failed to open CSV file", err)
	}
	s.csv = f

	ctx, cancel := context.WithCancel(context.Background())
	s.cancelWrite = cancel
	go func() {
		<-ctx.Done()
		close(s.results)
	}()
	go s.processResults()

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

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Collector.Port))
	if err != nil {
		log.Fatal("failed to bind server to port", err, slog.Int("port", int(config.Collector.Port)))
	}

	log.Info(
		"collector server is listening",
		slog.Int("port", int(config.Collector.Port)),
	)

	s.server = &http.Server{
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
			log.Info("shutting down collector server, hrtester collect process will terminate")

			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := s.server.Shutdown(ctx); err != nil {
					log.Warn("server forced to shut down", slog.Any("err", err))
				}
				s.cancelWrite()
			}()
		},
	)
}

////////////////////////////////////////////////////////////////////////////////

func (s *service) handleDefault(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			http.Error(w, "Unsupported Content-Type", http.StatusUnsupportedMediaType)
			return
		}
		if err := r.ParseForm(); err != nil {
			log.Debug("invalid form data", slog.Any("err", err))
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}
		select {
		case s.results <- shared.NewTestResult(r.Form):
		default:
			log.Warn("dropping result due to full buffer")
		}
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
	case "/__service/terminate", "/__service/terminate/":
		switch r.Method {
		case http.MethodPost:
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

func (s *service) processResults() {
	defer close(s.terminated)

	w := csv.NewWriter(s.csv)
	ticker := time.NewTicker(FlushInterval * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case r, ok := <-s.results:
			if !ok {
				w.Flush()
				if err := s.csv.Close(); err != nil {
					log.Error("failed to close CSV file", err)
				}
				return
			}
			if err := w.Write(r.Slice()); err != nil {
				log.Error("failed to write result", err)
			}
		case <-ticker.C:
			w.Flush()
		}
	}
}

////////////////////////////////////////////////////////////////////////////////
