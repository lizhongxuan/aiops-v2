package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"runner/logging"
)

const HeaderTraceID = "X-Trace-Id"

type contextKey string

const traceIDContextKey contextKey = "trace_id"

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			allowed := false
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Runner-Token, X-Trace-Id")
			w.Header().Set("Access-Control-Expose-Headers", "X-Trace-Id")
			w.Header().Set("Access-Control-Max-Age", "86400")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	wrapped := h
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}
	return wrapped
}

func TraceAndAccessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := strings.TrimSpace(r.Header.Get(HeaderTraceID))
		if traceID == "" {
			traceID = newTraceID()
		}
		w.Header().Set(HeaderTraceID, traceID)

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		cost := time.Since(start)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}

		logging.L().Info("http request",
			zap.String("trace_id", traceID),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("query", r.URL.RawQuery),
			zap.String("remote_addr", r.RemoteAddr),
			zap.Int("status", rec.status),
			zap.Int("bytes", rec.bytes),
			zap.Duration("duration", cost),
		)
	})
}

func AuthMiddleware(enabled bool, token string) func(http.Handler) http.Handler {
	trimmedToken := strings.TrimSpace(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}
			if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			auth := strings.TrimSpace(r.Header.Get("Authorization"))
			if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				auth = strings.TrimSpace(auth[len("bearer "):])
			}
			headerToken := strings.TrimSpace(r.Header.Get("X-Runner-Token"))
			queryToken := streamQueryToken(r)
			if auth == trimmedToken || headerToken == trimmedToken || queryToken == trimmedToken {
				next.ServeHTTP(w, r)
				return
			}

			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		})
	}
}

func newTraceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "trace-fallback"
	}
	return "trace-" + hex.EncodeToString(buf)
}

func streamQueryToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	path := strings.TrimSpace(r.URL.Path)
	if !strings.HasPrefix(path, "/api/v1/runs/") || !strings.HasSuffix(path, "/events") {
		return ""
	}
	if token := strings.TrimSpace(r.URL.Query().Get("access_token")); token != "" {
		return token
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}
