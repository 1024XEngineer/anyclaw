package gateway

import (
	"net/http"
	"strings"

	agentstore "github.com/1024XEngineer/anyclaw/pkg/capability/catalogs"
)

func (s *Server) handleV2Store(w http.ResponseWriter, r *http.Request) {
	if s.storeModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store not available"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	filter := agentstore.StoreFilter{
		Category: r.URL.Query().Get("category"),
		Tag:      r.URL.Query().Get("tag"),
		Keyword:  r.URL.Query().Get("q"),
	}

	if installedStr := r.URL.Query().Get("installed"); installedStr != "" {
		installed := installedStr == "true"
		filter.Installed = &installed
	}

	packages := s.storeModule.List(filter)
	writeJSON(w, http.StatusOK, packages)
}

func (s *Server) handleV2StoreByID(w http.ResponseWriter, r *http.Request) {
	if s.storeModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store not available"})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/v2/store/")
	if id == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"categories": s.storeModule.GetCategories(),
			"tags":       s.storeModule.GetTags(),
		})
		return
	}

	parts := strings.SplitN(id, "/", 2)
	pkgID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "install" && r.Method == http.MethodPost:
		if err := s.storeModule.Install(pkgID); err != nil {
			code := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				code = http.StatusNotFound
			}
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "installed", "id": pkgID})

	case action == "uninstall" && r.Method == http.MethodPost:
		if err := s.storeModule.Uninstall(pkgID); err != nil {
			code := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				code = http.StatusNotFound
			}
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled", "id": pkgID})

	case action == "" && r.Method == http.MethodGet:
		pkg, err := s.storeModule.Get(pkgID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, pkg)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
