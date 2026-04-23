package gateway

import (
	"net/http"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
)

func (s *Server) handleMarketSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := plugin.SearchFilter{
		Query:     r.URL.Query().Get("q"),
		Author:    r.URL.Query().Get("author"),
		SortBy:    r.URL.Query().Get("sort"),
		SortOrder: r.URL.Query().Get("order"),
		Limit:     parseIntParam(r.URL.Query().Get("limit"), 50),
		Offset:    parseIntParam(r.URL.Query().Get("offset"), 0),
	}

	if tags := r.URL.Query().Get("tags"); tags != "" {
		filter.Tags = strings.Split(tags, ",")
	}

	results, err := s.marketStore.Search(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error(), "plugins": []any{}})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"plugins": results,
		"total":   len(results),
	})
}

func (s *Server) handleMarketPlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "plugin id required", http.StatusBadRequest)
		return
	}

	listing, err := s.marketStore.GetPlugin(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, listing)
}

func (s *Server) handleMarketPluginAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/market/plugins/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "plugin id required", http.StatusBadRequest)
		return
	}
	pluginID := parts[0]

	ctx := r.Context()
	switch r.Method {
	case http.MethodPost:
		if len(parts) > 1 {
			switch parts[1] {
			case "install":
				version := r.URL.Query().Get("version")
				result, err := s.marketStore.Install(ctx, pluginID, version)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, result)
				return
			case "update":
				result, err := s.marketStore.Update(ctx, pluginID)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, result)
				return
			case "uninstall":
				result, err := s.marketStore.Uninstall(pluginID)
				if err != nil {
					writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, result)
				return
			case "rollback":
				targetVersion := r.URL.Query().Get("version")
				if targetVersion == "" {
					http.Error(w, "version query param required for rollback", http.StatusBadRequest)
					return
				}
				result, err := s.marketStore.Rollback(ctx, pluginID, targetVersion)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, result)
				return
			}
		}
		http.Error(w, "unknown action", http.StatusBadRequest)
	case http.MethodGet:
		if len(parts) > 1 && parts[1] == "versions" {
			versions, err := s.marketStore.GetVersions(ctx, pluginID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
			return
		}
		if len(parts) > 1 && parts[1] == "history" {
			history := s.marketStore.GetInstallHistory(pluginID)
			writeJSON(w, http.StatusOK, map[string]any{"history": history})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMarketInstalled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	records := s.marketStore.ListInstalled()
	writeJSON(w, http.StatusOK, map[string]any{"installed": records})
}

func (s *Server) handleMarketCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	categories := []string{
		"channel", "tool", "mcp", "skill", "app",
		"model-provider", "speech", "context-engine",
		"node", "surface", "ingress", "workflow-pack",
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": categories})
}
