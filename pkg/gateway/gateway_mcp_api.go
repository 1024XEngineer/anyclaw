package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) handleMCPServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mcpRegistry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"servers": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": s.mcpRegistry.Status()})
}

func (s *Server) handleMCPTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mcpRegistry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"tools": map[string]any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": s.mcpRegistry.AllTools()})
}

func (s *Server) handleMCPResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mcpRegistry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"resources": map[string]any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resources": s.mcpRegistry.AllResources()})
}

func (s *Server) handleMCPPrompts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mcpRegistry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"prompts": map[string]any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prompts": s.mcpRegistry.AllPrompts()})
}

func (s *Server) handleMCPCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Server   string         `json:"server"`
		Tool     string         `json:"tool"`
		Resource string         `json:"resource"`
		Prompt   string         `json:"prompt"`
		Args     map[string]any `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if s.mcpRegistry == nil {
		http.Error(w, "MCP not configured", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	var result any
	var err error

	if req.Tool != "" {
		result, err = s.mcpRegistry.CallTool(ctx, req.Server, req.Tool, req.Args)
	} else if req.Resource != "" {
		result, err = s.mcpRegistry.ReadResource(ctx, req.Server, req.Resource)
	} else if req.Prompt != "" {
		strArgs := make(map[string]string)
		for k, v := range req.Args {
			strArgs[k] = fmt.Sprintf("%v", v)
		}
		result, err = s.mcpRegistry.GetPrompt(ctx, req.Server, req.Prompt, strArgs)
	} else {
		http.Error(w, "must specify tool, resource, or prompt", http.StatusBadRequest)
		return
	}

	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (s *Server) handleMCPServerAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/mcp/servers/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "server name required", http.StatusBadRequest)
		return
	}
	serverName := parts[0]

	if s.mcpRegistry == nil {
		http.Error(w, "MCP not configured", http.StatusServiceUnavailable)
		return
	}

	client, ok := s.mcpRegistry.Get(serverName)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	ctx := r.Context()
	switch r.Method {
	case http.MethodPost:
		if len(parts) > 1 {
			switch parts[1] {
			case "connect":
				if err := client.Connect(ctx); err != nil {
					writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"status": "connected"})
				return
			case "disconnect":
				client.Close()
				writeJSON(w, http.StatusOK, map[string]any{"status": "disconnected"})
				return
			}
		}
		http.Error(w, "unknown action", http.StatusBadRequest)
	case http.MethodGet:
		status := map[string]any{
			"name":      serverName,
			"connected": client.IsConnected(),
		}
		if client.IsConnected() {
			status["tools"] = len(client.ListTools())
			status["resources"] = len(client.ListResources())
			status["prompts"] = len(client.ListPrompts())
		}
		writeJSON(w, http.StatusOK, status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
