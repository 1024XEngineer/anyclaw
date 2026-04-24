package sessionbridge

import (
	"context"
	"fmt"

	sessionrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/sessionrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

// Service bridges API/WS session execution requests into the runtime session runner.
type Service struct {
	Runner *sessionrunner.Manager
}

// RunMessage executes one session message with the provided run options.
func (s Service) RunMessage(ctx context.Context, sessionID string, title string, message string, opts sessionrunner.RunOptions) (string, *state.Session, error) {
	if s.Runner == nil {
		return "", nil, fmt.Errorf("session runner not initialized")
	}
	result, err := s.Runner.Run(ctx, sessionrunner.RunRequest{
		SessionID: sessionID,
		Title:     title,
		Message:   message,
		Options:   opts,
	})
	if result == nil {
		return "", nil, err
	}
	return result.Response, result.Session, err
}
