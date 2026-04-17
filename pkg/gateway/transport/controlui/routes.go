package controlui

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Options struct {
	BasePath string
	Root     string
}

func RegisterRoutes(mux *http.ServeMux, opts Options) {
	if mux == nil {
		return
	}

	handler := routeHandler{opts: opts}

	basePath := strings.TrimSpace(opts.BasePath)
	if basePath == "" {
		basePath = "/dashboard"
	}

	controlPaths := []string{basePath, "/dashboard", "/control"}
	seen := map[string]bool{}
	for _, route := range controlPaths {
		route = strings.TrimSpace(route)
		if route == "" || seen[route] {
			continue
		}
		seen[route] = true
		mux.HandleFunc(route, handler.handleControlUI)
		mux.HandleFunc(route+"/", handler.handleControlUI)
	}

	mux.HandleFunc("/market", handler.handleMarketUI)
	mux.HandleFunc("/market/", handler.handleMarketUI)
	mux.HandleFunc("/discovery", handler.handleDiscoveryUI)
	mux.HandleFunc("/discovery/", handler.handleDiscoveryUI)
}

type routeHandler struct {
	opts Options
}

func (h routeHandler) controlUIRoot() string {
	candidates := []string{
		strings.TrimSpace(os.Getenv("ANYCLAW_CONTROL_UI_ROOT")),
		strings.TrimSpace(h.opts.Root),
		"dist/control-ui",
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func (h routeHandler) handleControlUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if root := h.controlUIRoot(); root != "" {
		if h.tryServeControlUIAsset(w, r, root) {
			return
		}
		indexPath := filepath.Join(root, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}
	}
	http.Error(w, "control UI not found", http.StatusNotFound)
}

func (h routeHandler) tryServeControlUIAsset(w http.ResponseWriter, r *http.Request, root string) bool {
	controlPaths := []string{
		strings.TrimSpace(h.opts.BasePath),
		"/dashboard",
		"/control",
	}

	requestPath := path.Clean(r.URL.Path)
	for _, base := range controlPaths {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		base = path.Clean(base)
		if requestPath == base || requestPath == base+"/" {
			return false
		}
		prefix := base + "/"
		if !strings.HasPrefix(requestPath, prefix) {
			continue
		}
		rel := strings.TrimPrefix(requestPath, prefix)
		if rel == "" || rel == "." {
			return false
		}
		target := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(target)
		if err != nil || info.IsDir() {
			return false
		}
		http.ServeFile(w, r, target)
		return true
	}
	return false
}

func (h routeHandler) handleMarketUI(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile("ui/market/index.html")
	if err != nil {
		root := h.controlUIRoot()
		if root != "" {
			data, err = os.ReadFile(filepath.Join(root, "market", "index.html"))
		}
	}
	if err != nil {
		http.Error(w, "market UI not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (h routeHandler) handleDiscoveryUI(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile("ui/discovery/index.html")
	if err != nil {
		root := h.controlUIRoot()
		if root != "" {
			data, err = os.ReadFile(filepath.Join(root, "discovery", "index.html"))
		}
	}
	if err != nil {
		http.Error(w, "discovery UI not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
