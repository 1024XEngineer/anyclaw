package surface

import (
	"encoding/json"
	"net/http"
	"strings"

	gatewaygovernance "github.com/1024XEngineer/anyclaw/pkg/gateway/governance"
)

// TransportRef describes the protocol-facing request metadata seen by the gateway.
type TransportRef struct {
	Protocol     string
	Method       string
	Path         string
	ConnectionID string
	ClientID     string
}

// Service owns protocol-facing projection into gateway internal ingress objects.
type Service struct{}

// ChannelInput is the protocol-surface view of a channel message.
type ChannelInput struct {
	Source    string
	SessionID string
	Message   string
	Meta      map[string]string
}

// SignedWebhookInput is the protocol-surface view of a signed webhook message.
type SignedWebhookInput struct {
	SessionID string
	Title     string
	Message   string
	Meta      map[string]string
}

// WriteOutput is the protocol-surface response contract for HTTP-like transports.
type WriteOutput struct {
	StatusCode int
	Payload    any
	Headers    map[string]string
}

// HTTPTransport builds a protocol reference from an incoming HTTP request.
func HTTPTransport(r *http.Request, connectionID string) TransportRef {
	if r == nil {
		return TransportRef{Protocol: "http", ConnectionID: strings.TrimSpace(connectionID)}
	}
	clientID := strings.TrimSpace(r.Header.Get("X-Client-ID"))
	if clientID == "" {
		clientID = strings.TrimSpace(r.Header.Get("X-Device-ID"))
	}
	protocol := "http"
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		protocol = "ws"
	}
	return TransportRef{
		Protocol:     protocol,
		Method:       strings.ToUpper(strings.TrimSpace(r.Method)),
		Path:         strings.TrimSpace(r.URL.Path),
		ConnectionID: strings.TrimSpace(connectionID),
		ClientID:     clientID,
	}
}

// Write writes a structured gateway response through the protocol surface.
func (Service) Write(w http.ResponseWriter, output WriteOutput) error {
	if output.StatusCode == 0 {
		output.StatusCode = http.StatusOK
	}
	for key, value := range output.Headers {
		if trimmedKey := strings.TrimSpace(key); trimmedKey != "" {
			w.Header().Set(trimmedKey, value)
		}
	}
	if strings.TrimSpace(w.Header().Get("Content-Type")) == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(output.StatusCode)
	return json.NewEncoder(w).Encode(output.Payload)
}

// WriteError writes a structured error response through the protocol surface.
func (s Service) WriteError(w http.ResponseWriter, statusCode int, message string) error {
	if strings.TrimSpace(message) == "" {
		message = http.StatusText(statusCode)
	}
	return s.Write(w, WriteOutput{
		StatusCode: statusCode,
		Payload: map[string]string{
			"error": strings.TrimSpace(message),
		},
	})
}

// BuildChannelRawRequest translates channel ingress facts into the gateway raw request contract.
func (Service) BuildChannelRawRequest(input ChannelInput) gatewaygovernance.RawRequest {
	meta := cloneStringMap(input.Meta)
	return gatewaygovernance.RawRequest{
		SourceType:          "channel",
		EntryPoint:          "channel",
		ChannelID:           strings.TrimSpace(input.Source),
		SessionID:           strings.TrimSpace(input.SessionID),
		Message:             input.Message,
		ActorUserID:         strings.TrimSpace(meta["user_id"]),
		ActorDisplayName:    firstNonEmpty(meta["username"], meta["user_name"], meta["display_name"]),
		ThreadID:            strings.TrimSpace(meta["thread_id"]),
		GroupID:             firstNonEmpty(meta["group_id"], meta["guild_id"]),
		IsGroup:             strings.EqualFold(strings.TrimSpace(meta["is_group"]), "true"),
		RequestedAgentName:  firstNonEmpty(meta["agent_name"], meta["assistant_name"], meta["agent"], meta["assistant"]),
		Metadata:            meta,
		DeliveryReplyTarget: strings.TrimSpace(meta["reply_target"]),
	}
}

// BuildSignedWebhookRawRequest translates a signed webhook payload into the gateway raw request contract.
func (Service) BuildSignedWebhookRawRequest(input SignedWebhookInput) gatewaygovernance.RawRequest {
	meta := cloneStringMap(input.Meta)
	if strings.TrimSpace(input.Title) != "" {
		meta["title_hint"] = strings.TrimSpace(input.Title)
	}
	return gatewaygovernance.RawRequest{
		SourceType:         "webhook",
		EntryPoint:         "webhook",
		ChannelID:          "webhook",
		SessionID:          strings.TrimSpace(input.SessionID),
		Message:            input.Message,
		TitleHint:          strings.TrimSpace(input.Title),
		RequestedSessionID: strings.TrimSpace(input.SessionID),
		Metadata:           meta,
	}
}

// WriteJSON is the shared JSON response writer used by the protocol surface.
func WriteJSON(w http.ResponseWriter, statusCode int, value any) error {
	return Service{}.Write(w, WriteOutput{
		StatusCode: statusCode,
		Payload:    value,
	})
}

// WriteError is the shared JSON error writer used by the protocol surface.
func WriteError(w http.ResponseWriter, statusCode int, message string) error {
	return Service{}.WriteError(w, statusCode, message)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return map[string]string{}
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
