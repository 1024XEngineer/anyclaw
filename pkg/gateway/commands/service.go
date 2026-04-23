package commands

import (
	"fmt"
	"strings"
)

// Request is the normalized control-plane command contract consumed by command intake.
type Request struct {
	RequestID  string
	Method     string
	ResourceID string
	Params     map[string]string
}

// Dispatch is the routing result returned by command intake for one command request.
type Dispatch struct {
	Kind     string
	Target   string
	Action   string
	Metadata map[string]string
}

// Result is the normalized command completion payload returned to protocol handlers.
type Result struct {
	RequestID string
	Status    string
	Payload   map[string]string
	StreamID  string
}

// Service owns gateway command-intake helpers.
type Service struct{}

// Dispatch classifies one control-plane command into query, mutate, or ingress.
func (s Service) Dispatch(req Request) (Dispatch, error) {
	method := strings.TrimSpace(strings.ToLower(req.Method))
	if method == "" {
		return Dispatch{}, fmt.Errorf("command method is required")
	}

	switch method {
	case "chat.send", "chat.v2.send":
		return Dispatch{
			Kind:   "ingress",
			Target: "ingress",
			Action: "chat.send",
			Metadata: map[string]string{
				"command_method": method,
			},
		}, nil
	case "tasks.v2.create":
		return Dispatch{
			Kind:   "mutate",
			Target: "tasks",
			Action: "create",
			Metadata: map[string]string{
				"command_method": method,
			},
		}, nil
	}

	parts := strings.Split(method, ".")
	if len(parts) >= 2 {
		target := strings.TrimSpace(parts[0])
		action := strings.TrimSpace(parts[len(parts)-1])
		kind := classifyAction(action)
		if target != "" && action != "" {
			return Dispatch{
				Kind:   kind,
				Target: target,
				Action: action,
				Metadata: map[string]string{
					"command_method": method,
				},
			}, nil
		}
	}

	return Dispatch{}, fmt.Errorf("unsupported command method: %s", req.Method)
}

// Complete builds the normalized command result contract for one handled command.
func (s Service) Complete(req Request, payload map[string]string, status string) Result {
	cloned := make(map[string]string, len(payload))
	for key, value := range payload {
		cloned[key] = value
	}
	return Result{
		RequestID: strings.TrimSpace(req.RequestID),
		Status:    strings.TrimSpace(status),
		Payload:   cloned,
	}
}

func classifyAction(action string) string {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "list", "get", "read", "status", "snapshot":
		return "query"
	default:
		return "mutate"
	}
}
