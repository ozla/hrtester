package middleware

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/ozla/hrtester/internal/log"
)

////////////////////////////////////////////////////////////////////////////////

func DrainAndCloseHandler(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			next(w, r)
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
		},
	)
}

func DebugHandler(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			log.Debug(
				"received request",
				slog.String("remoteAddr", r.RemoteAddr),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)
			next(w, r)
		},
	)
}

////////////////////////////////////////////////////////////////////////////////

func WrapHandlerFuncs(h http.HandlerFunc, wraps ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	chain := h
	for i := len(wraps) - 1; i >= 0; i-- {
		chain = wraps[i](chain)
	}
	return chain
}

////////////////////////////////////////////////////////////////////////////////
