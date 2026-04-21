package events

import (
	"net/http"

	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

// Service is the gateway-facing facade for event and audit flows.
type Service struct {
	store *state.Store
	bus   *state.EventBus
}

// NewService creates an event/audit facade around the gateway store and bus.
func NewService(store *state.Store, bus *state.EventBus) Service {
	return Service{store: store, bus: bus}
}

// AppendEvent records and publishes a gateway event.
func (s Service) AppendEvent(eventType string, sessionID string, payload map[string]any) {
	AppendEvent(s.store, s.bus, eventType, sessionID, payload)
}

// AppendAudit records an audit event.
func (s Service) AppendAudit(user *gatewayauth.User, action string, target string, meta map[string]any) {
	AppendAudit(s.store, user, action, target, meta)
}

// SessionCreatedEventPayload returns the canonical payload for session.created.
func (s Service) SessionCreatedEventPayload(session *state.Session) map[string]any {
	return SessionCreatedEventPayload(session)
}

// HandleList serves the event list endpoint.
func (s Service) HandleList(w http.ResponseWriter, r *http.Request) {
	HandleList(w, r, s.store)
}

// HandleStream serves the SSE event stream endpoint.
func (s Service) HandleStream(w http.ResponseWriter, r *http.Request) {
	HandleStream(w, r, s.store, s.bus)
}
