package api

import (
	"net/http"
	"strings"

	"go.uber.org/zap"
	"runner/logging"
)

func auditLog(r *http.Request, action, resource string, payload map[string]any) {
	if r == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	logging.L().Info("audit",
		zap.String("action", strings.TrimSpace(action)),
		zap.String("resource", strings.TrimSpace(resource)),
		zap.String("actor", actorFromRequest(r)),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.Any("payload", payload),
	)
}

func actorFromRequest(r *http.Request) string {
	for _, key := range []string{"X-Actor", "X-User", "X-Requester"} {
		if value := strings.TrimSpace(r.Header.Get(key)); value != "" {
			return value
		}
	}
	return strings.TrimSpace(r.RemoteAddr)
}
