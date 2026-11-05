package server

import (
	"log/slog"
	"net/http"
	"time"
)

// loggingWriter captures the response status for logging.
type loggingWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// logRequests logs each HTTP request (method, path, status, duration) at Debug
// level so mobile streaming noise stays out of the default logs.
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if log == nil {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		lw := &loggingWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)
		log.Debug("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", lw.status,
			"dur", time.Since(start).Round(time.Millisecond).String(),
		)
	})
}
