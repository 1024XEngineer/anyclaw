package intake

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
	taskrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type SessionRunnerFunc func(ctx context.Context, sessionID string, title string, message string) (string, *state.Session, error)

type SignedIngressAPI struct {
	Secret                  string
	RunSessionMessage       SessionRunnerFunc
	SessionApprovalResponse func(sessionID string) map[string]any
	CurrentUser             CurrentUserFunc
	AppendAudit             AuditRecorder
	AppendEvent             EventRecorder
}

func (api SignedIngressAPI) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := strings.TrimSpace(api.Secret)
	if secret == "" {
		http.Error(w, "webhook secret not configured", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	provided := strings.TrimSpace(r.Header.Get("X-AnyClaw-Signature"))
	if !VerifySignature(secret, body, provided) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	var req struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
		Title     string `json:"title"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}
	if api.RunSessionMessage == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session runner not available"})
		return
	}
	response, session, err := api.RunSessionMessage(r.Context(), req.SessionID, req.Title, req.Message)
	if err != nil {
		if errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
			api.appendAudit(api.currentUser(r.Context()), "ingress.web.accepted", req.SessionID, map[string]any{"status": "waiting_approval"})
			if api.SessionApprovalResponse != nil {
				writeJSON(w, http.StatusAccepted, api.SessionApprovalResponse(req.SessionID))
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{"status": "waiting_approval"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	api.appendEvent("ingress.web.accepted", session.ID, map[string]any{"signed": true})
	api.appendAudit(api.currentUser(r.Context()), "ingress.web.accepted", session.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"response": response, "session": session})
}

func VerifySignature(secret string, body []byte, provided string) bool {
	if strings.TrimSpace(provided) == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := fmt.Sprintf("sha256=%x", mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(provided)))
}

func (api SignedIngressAPI) currentUser(ctx context.Context) *gatewayauth.User {
	if api.CurrentUser == nil {
		return nil
	}
	return api.CurrentUser(ctx)
}

func (api SignedIngressAPI) appendAudit(user *gatewayauth.User, action string, target string, meta map[string]any) {
	if api.AppendAudit == nil {
		return
	}
	api.AppendAudit(user, action, target, meta)
}

func (api SignedIngressAPI) appendEvent(eventType string, sessionID string, payload map[string]any) {
	if api.AppendEvent == nil {
		return
	}
	api.AppendEvent(eventType, sessionID, payload)
}
