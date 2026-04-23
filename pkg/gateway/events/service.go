package events

import (
	"fmt"
	"sync/atomic"
	"time"

	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

var auditIDCounter uint64

func AppendEvent(store *state.Store, bus *state.EventBus, eventType string, sessionID string, payload map[string]any) {
	event := state.NewEvent(eventType, sessionID, payload)
	if store != nil {
		_ = store.AppendEvent(event)
	}
	if bus != nil {
		bus.Publish(event)
	}
}

func AppendAudit(store *state.Store, user *gatewayauth.User, action string, target string, meta map[string]any) {
	if store == nil {
		return
	}
	actor := "anonymous"
	role := ""
	if user != nil {
		actor = user.Name
		role = user.Role
	}
	_ = store.AppendAudit(&state.AuditEvent{
		ID:        uniqueAuditID("aud"),
		Actor:     actor,
		Role:      role,
		Action:    action,
		Target:    target,
		Timestamp: time.Now().UTC(),
		Meta:      meta,
	})
}

func SessionCreatedEventPayload(session *state.Session) map[string]any {
	org, project, workspace := state.SessionExecutionHierarchy(session)
	return map[string]any{
		"title":     session.Title,
		"org":       org,
		"project":   project,
		"workspace": workspace,
	}
}

func uniqueAuditID(prefix string) string {
	seq := atomic.AddUint64(&auditIDCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), seq)
}
