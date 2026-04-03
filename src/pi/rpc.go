package pi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/runtime"
)

type RPCServer struct {
	addr       string
	httpServer *http.Server
	workDir    string
	appCfg     *config.Config
	mu         sync.RWMutex
	running    bool
}

type RPCRequest struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id,omitempty"`
	Command   string `json:"command"`
	Input     string `json:"input,omitempty"`
}

type RPCResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func NewRPCServer(addr string, workDir string, appCfg *config.Config) *RPCServer {
	return &RPCServer{
		addr:    addr,
		workDir: workDir,
		appCfg:  appCfg,
	}
}

func (s *RPCServer) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat", s.handleChat)
	mux.HandleFunc("/v1/sessions", s.handleSessions)
	mux.HandleFunc("/v1/sessions/", s.handleSessionByID)
	mux.HandleFunc("/v1/agents", s.handleAgents)
	mux.HandleFunc("/v1/agents/", s.handleAgentByID)
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/", s.handleRoot)

	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	err := s.httpServer.ListenAndServe()
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	return err
}

func (s *RPCServer) Stop() error {
	s.mu.RLock()
	if !s.running || s.httpServer == nil {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()
	return s.httpServer.Shutdown(context.Background())
}

func (s *RPCServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode(RPCResponse{Success: true, Data: map[string]interface{}{
		"status": "ok",
		"agents": len(ListAllPiAgents()),
	}})
}

func (s *RPCServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":    "AnyClaw Pi Agent RPC",
		"version": runtime.Version,
		"endpoints": []string{
			"/v1/chat",
			"/v1/sessions",
			"/v1/agents",
			"/v1/health",
		},
	})
}

func (s *RPCServer) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: "invalid request"})
		return
	}

	if req.UserID == "" {
		json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: "user_id required"})
		return
	}

	userID := UserID(req.UserID)
	pi, ok := GetPiAgent(userID)
	if !ok {
		cfg := defaultConfig
		var err error
		pi, err = NewPiAgent(userID, cfg, s.appCfg, s.workDir)
		if err != nil {
			json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: err.Error()})
			return
		}
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	response, err := pi.RunSession(r.Context(), sessionID, req.Input)
	if err != nil {
		json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(RPCResponse{
		Success: true,
		Data: map[string]interface{}{
			"response":   response,
			"session_id": sessionID,
			"user_id":    req.UserID,
		},
	})
}

func (s *RPCServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: "user_id required"})
		return
	}

	pi, ok := GetPiAgent(UserID(userID))
	if !ok {
		json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: "user not found"})
		return
	}

	if r.Method == http.MethodGet {
		sessions := pi.ListSessions()
		data := make([]map[string]interface{}, 0, len(sessions))
		for _, sess := range sessions {
			data = append(data, map[string]interface{}{
				"id":            sess.ID,
				"user_id":       sess.UserID,
				"created_at":    sess.CreatedAt,
				"message_count": len(sess.History) / 2,
			})
		}
		json.NewEncoder(w).Encode(RPCResponse{Success: true, Data: data})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (s *RPCServer) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: "user_id required"})
		return
	}

	pi, ok := GetPiAgent(UserID(userID))
	if !ok {
		json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: "user not found"})
		return
	}

	sessionID := r.URL.Path[len("/v1/sessions/"):]
	if sessionID == "" {
		json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: "session_id required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		history := pi.GetHistory(sessionID)
		json.NewEncoder(w).Encode(RPCResponse{Success: true, Data: history})

	case http.MethodDelete:
		if err := pi.ClearHistory(sessionID); err != nil {
			json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: err.Error()})
			return
		}
		json.NewEncoder(w).Encode(RPCResponse{Success: true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *RPCServer) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		agents := ListAllPiAgents()
		data := make([]map[string]interface{}, 0, len(agents))
		for _, a := range agents {
			data = append(data, map[string]interface{}{
				"id":            a.ID(),
				"name":          a.Name(),
				"session_count": len(a.ListSessions()),
				"privacy_mode":  a.IsPrivacyMode(),
			})
		}
		json.NewEncoder(w).Encode(RPCResponse{Success: true, Data: data})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (s *RPCServer) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Path[len("/v1/agents/"):]
	if userID == "" {
		json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: "user_id required"})
		return
	}

	pi, ok := GetPiAgent(UserID(userID))
	if !ok {
		json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: "agent not found"})
		return
	}

	if r.Method == http.MethodDelete {
		if err := DeletePiAgent(UserID(userID)); err != nil {
			json.NewEncoder(w).Encode(RPCResponse{Success: false, Error: err.Error()})
			return
		}
		json.NewEncoder(w).Encode(RPCResponse{Success: true})
		return
	}

	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(RPCResponse{Success: true, Data: map[string]interface{}{
			"id":           pi.ID(),
			"name":         pi.Name(),
			"user_dir":     pi.UserDir(),
			"privacy_mode": pi.IsPrivacyMode(),
			"sessions":     len(pi.ListSessions()),
		}})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func generateSessionID() string {
	return fmt.Sprintf("sess-%d", time.Now().UnixMilli())
}
