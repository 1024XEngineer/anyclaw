package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/gateway/auth"
	"github.com/anyclaw/anyclaw/pkg/gateway/middleware"
	"github.com/anyclaw/anyclaw/pkg/gateway/session"
	"github.com/anyclaw/anyclaw/pkg/gateway/ws"
)

type Server struct {
	config     *config.Config
	sessionMgr *session.Manager
	auth       *auth.Middleware
	rateLimit  *middleware.RateLimiter
	wsHub      *ws.Hub
	handlers   map[string]http.HandlerFunc
}

func NewServer(cfg *config.Config, sessionMgr *session.Manager, wsHub *ws.Hub) *Server {
	return &Server{
		config:     cfg,
		sessionMgr: sessionMgr,
		auth:       auth.NewMiddleware(cfg),
		rateLimit:  middleware.NewRateLimiter(100, 60),
		wsHub:      wsHub,
		handlers:   make(map[string]http.HandlerFunc),
	}
}

func (s *Server) RegisterHandler(path string, fn http.HandlerFunc) {
	s.handlers[path] = fn
}

func (s *Server) HandleHealth(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) HandleStatus(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"version":  "2026.3.13",
		"uptime":   time.Since(time.Now()),
		"gateway":  "running",
		"channels": []string{},
		"sessions": s.sessionMgr.Count(),
		"runtimes": 0,
	})
}

func (s *Server) HandleChat(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	var payload struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id,omitempty"`
		Agent     string `json:"agent,omitempty"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"response": "Echo: " + payload.Message,
		"session":  payload.SessionID,
	})
}

func (s *Server) HandleChannels(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"channels": []map[string]string{
			{"name": "telegram", "status": "inactive"},
			{"name": "discord", "status": "inactive"},
			{"name": "slack", "status": "inactive"},
		},
	})
}

func (s *Server) HandleSessions(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"sessions": s.sessionMgr.List(),
	})
}

func (s *Server) HandleWebSocket(w http.ResponseWriter, req *http.Request) {
	s.wsHub.HandleConnection(w, req)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path

	if fn, ok := s.handlers[path]; ok {
		fn(w, req)
		return
	}

	if strings.HasPrefix(path, "/ws") {
		s.HandleWebSocket(w, req)
		return
	}

	http.NotFound(w, req)
}

func Start(ctx context.Context, cfg *config.Config, sessionMgr *session.Manager) error {
	addr := fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)

	wsHub := ws.NewHub()
	go wsHub.Run()

	server := NewServer(cfg, sessionMgr, wsHub)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", server.HandleHealth)
	mux.HandleFunc("/status", server.HandleStatus)
	mux.HandleFunc("/chat", server.HandleChat)
	mux.HandleFunc("/channels", server.HandleChannels)
	mux.HandleFunc("/sessions", server.HandleSessions)
	mux.HandleFunc("/ws", server.HandleWebSocket)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	fmt.Printf("Gateway started at http://%s\n", addr)
	<-ctx.Done()

	return httpServer.Shutdown(context.Background())
}
