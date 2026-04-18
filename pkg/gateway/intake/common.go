package intake

import (
	"context"
	"encoding/json"
	"net/http"

	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
)

type CurrentUserFunc func(context.Context) *gatewayauth.User
type AuditRecorder func(user *gatewayauth.User, action string, target string, meta map[string]any)
type EventRecorder func(eventType string, sessionID string, payload map[string]any)

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
