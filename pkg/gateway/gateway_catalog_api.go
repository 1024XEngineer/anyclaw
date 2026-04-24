package gateway

import (
	"net/http"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
	routeingress "github.com/1024XEngineer/anyclaw/pkg/route/ingress"
)

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.channels == nil {
		writeJSON(w, http.StatusOK, []inputlayer.Status{})
		return
	}
	writeJSON(w, http.StatusOK, s.channels.Statuses())
}

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.plugins == nil {
		writeJSON(w, http.StatusOK, []plugin.Manifest{})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "plugins.read", "plugins", nil)
	writeJSON(w, http.StatusOK, s.plugins.List())
}

func (s *Server) handleRouting(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.mainRuntime.Config.Channels.Routing)
}

func (s *Server) handleRoutingAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, routeingress.AnalyzeRouting(s.mainRuntime.Config.Channels.Routing))
}

func (s *Server) handlePersonalityTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, config.BuiltinPersonalityTemplates)
}

func (s *Server) handleAssistantSkillCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.mainRuntime.Skills.Catalog())
}
