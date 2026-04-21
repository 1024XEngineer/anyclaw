package commands

import (
	"fmt"
	"strings"
)

// V2TaskCreateRequest is the normalized command-intake contract for /v2/tasks POST.
type V2TaskCreateRequest struct {
	Title          string
	Input          string
	Mode           string
	Assistant      string
	SessionID      string
	SelectedAgent  string
	SelectedAgents []string
	Sync           bool
}

// NewV2TaskCreateCommandRequest converts the /v2/tasks POST payload into the command-intake contract.
func NewV2TaskCreateCommandRequest(req V2TaskCreateRequest) Request {
	return Request{
		Method:     "tasks.v2.create",
		ResourceID: strings.TrimSpace(req.SessionID),
		Params: map[string]string{
			"title":          req.Title,
			"input":          req.Input,
			"mode":           req.Mode,
			"assistant":      req.Assistant,
			"session_id":     req.SessionID,
			"selected_agent": req.SelectedAgent,
			"sync":           fmt.Sprintf("%t", req.Sync),
		},
	}
}

// ValidateV2TaskCreate validates the minimum required fields for /v2/tasks POST.
func ValidateV2TaskCreate(req V2TaskCreateRequest) (string, error) {
	if strings.TrimSpace(req.Input) == "" {
		return "", fmt.Errorf("input is required")
	}
	mode := strings.TrimSpace(strings.ToLower(req.Mode))
	if mode == "" {
		mode = "single"
	}
	if mode != "single" && mode != "multi" && mode != "main" {
		return "", fmt.Errorf("mode must be 'single', 'multi', or 'main'")
	}
	return mode, nil
}
