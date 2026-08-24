package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"surveyrelease/internal/application"
)

type contextKey string

const requestIDKey contextKey = "request-id"

func requestIDFrom(ctx context.Context) string { v, _ := ctx.Value(requestIDKey).(string); return v }
func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}

type captureWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *captureWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *captureWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func accessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = newRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set("X-Request-Id", id)
		cw := &captureWriter{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(cw, r.WithContext(ctx))
		logger.Info("http_request", "requestId", id, "method", r.Method, "path", r.URL.Path, "status", cw.status, "bytes", cw.bytes, "durationMs", time.Since(start).Milliseconds())
	})
}

func recoverPanic(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("http_panic", "requestId", requestIDFrom(r.Context()), "error", recovered)
				writeProblem(w, r, &application.AppError{Kind: application.KindPersistence, Code: "panic", Message: "服务内部错误"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
