package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

const defaultGatewayAddress = "127.0.0.1:18789"

// Server is the gateway runtime shell. It intentionally keeps this PR's
// contract small so runtime packages can depend on gateway.New without pulling
// in the full gateway wiring layer.
type Server struct {
	mainRuntime any
	httpServer  *http.Server
	startedAt   time.Time
	addr        string
}

// Run starts the minimal gateway HTTP shell.
func (s *Server) Run(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("gateway server is nil")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"started_at": s.startedAt,
		})
	})

	s.startedAt = time.Now().UTC()
	s.httpServer = &http.Server{
		Addr:              s.address(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("gateway server failed: %w", err)
	}
}

func (s *Server) address() string {
	if s == nil || s.addr == "" {
		return defaultGatewayAddress
	}
	if _, _, err := net.SplitHostPort(s.addr); err != nil {
		return defaultGatewayAddress
	}
	return s.addr
}
