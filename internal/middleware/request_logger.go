package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

func NewRequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			args := make([]any, 0, 2)
			args = append(args,
				headerGroup(r),
				slog.String("method", r.Method),
				slog.String("uri", r.RequestURI),
				slog.String("host", r.Host),
			)
			logger.DebugContext(ctx, "starting request", args...)
			if len(r.Pattern) > 0 {
				args = append(args, slog.String("pattern", r.Pattern))
			}
			sw := StatusWriter{w: w, Status: http.StatusOK}
			next.ServeHTTP(&sw, r)
			args = append(args, slog.Int("status", sw.Status))
			logger.InfoContext(ctx, "finished request", args...)
		})
	}
}

type StatusWriter struct {
	w      http.ResponseWriter
	Status int
}

func (sw *StatusWriter) WriteHeader(status int) {
	sw.Status = status
	sw.w.WriteHeader(status)
}

func (sw *StatusWriter) Header() http.Header { return sw.w.Header() }

func (sw *StatusWriter) Write(b []byte) (int, error) { return sw.w.Write(b) }

func headerGroup(r *http.Request) slog.Attr {
	args := make([]any, 0, len(r.Header))
	for k, v := range r.Header {
		args = append(args, slog.String(k, strings.Join(v, ",")))
	}
	return slog.Group("headers", args...)
}
