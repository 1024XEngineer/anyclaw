package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/canvas"
)

func (s *Server) initCanvas() {
	if s.app == nil {
		return
	}
	maxVersions := s.app.Config.Gateway.CanvasMaxVersions
	if maxVersions <= 0 {
		maxVersions = 20
	}
	store, err := canvas.NewStore(s.app.WorkDir, maxVersions)
	if err != nil {
		s.appendEvent("canvas.init.error", "", map[string]any{"error": err.Error()})
		return
	}
	s.canvasStore = store
	s.canvasHub = NewCanvasHub()
	go s.canvasHub.Run()
	s.appendEvent("canvas.init.ok", "", map[string]any{"dir": store.BaseDir()})
}

func (s *Server) handleCanvasList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.canvasStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "canvas not initialized"})
		return
	}
	entries := s.canvasStore.List()
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleCanvasPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.canvasStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "canvas not initialized"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	defer r.Body.Close()

	var req struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Content string `json:"content"`
		Type    string `json:"type"`
		Reset   bool   `json:"reset"`
		Agent   string `json:"agent"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}

	if req.Reset && req.ID != "" {
		if err := s.canvasStore.Reset(req.ID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
	}

	if strings.TrimSpace(req.Content) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content is required"})
		return
	}

	entry, err := s.canvasStore.Push(req.ID, req.Name, req.Content, req.Type, req.Agent)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.broadcastCanvasUpdate(entry)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"id":      entry.ID,
		"name":    entry.Name,
		"type":    entry.Type,
		"version": entry.Version,
		"updated": entry.UpdatedAt,
	})
}

func (s *Server) handleCanvasGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.canvasStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "canvas not initialized"})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/canvas/")
	if strings.TrimSpace(id) == "" {
		http.NotFound(w, r)
		return
	}

	entry, ok := s.canvasStore.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "canvas entry not found"})
		return
	}

	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) handleCanvasDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.canvasStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "canvas not initialized"})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/canvas/")
	if strings.TrimSpace(id) == "" {
		http.NotFound(w, r)
		return
	}

	if err := s.canvasStore.Delete(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (s *Server) handleCanvasVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.canvasStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "canvas not initialized"})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/canvas/")
	id = strings.TrimSuffix(id, "/versions")
	if strings.TrimSpace(id) == "" {
		http.NotFound(w, r)
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	versions := s.canvasStore.GetVersions(id, limit)
	writeJSON(w, http.StatusOK, versions)
}

func (s *Server) handleCanvasVersionGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.canvasStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "canvas not initialized"})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/canvas/")
	parts := strings.Split(path, "/versions/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	id := parts[0]
	version := 0
	fmt.Sscanf(parts[1], "%d", &version)

	v, ok := s.canvasStore.GetVersion(id, version)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version not found"})
		return
	}

	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleCanvasReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.canvasStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "canvas not initialized"})
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	if err := s.canvasStore.Reset(req.ID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": req.ID})
}

func (s *Server) handleCanvasRender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.canvasStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "canvas not initialized"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	defer r.Body.Close()

	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	entry, ok := s.canvasStore.Get(req.ID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "canvas entry not found"})
		return
	}

	if entry.Type == canvas.EntryTypeA2UI {
		doc, err := canvas.ParseA2UI(entry.Content)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid a2ui: " + err.Error()})
			return
		}
		renderer := canvas.NewA2UIRenderer()
		html, err := renderer.Render(doc)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(entry.Content))
}

func (s *Server) handleCanvasUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Path
	if path == "/canvas" || path == "/canvas/" {
		s.serveCanvasIndex(w, r)
		return
	}

	if strings.HasPrefix(path, "/canvas/a2ui/") {
		s.serveA2UIRendered(w, r)
		return
	}

	if strings.HasPrefix(path, "/canvas/view/") {
		s.serveCanvasView(w, r)
		return
	}

	s.serveCanvasIndex(w, r)
}

func (s *Server) serveCanvasIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(canvasIndexHTML))
}

func (s *Server) serveCanvasView(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/canvas/view/")
	if s.canvasStore == nil {
		http.Error(w, "canvas not initialized", http.StatusServiceUnavailable)
		return
	}

	entry, ok := s.canvasStore.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(entry.Content))
}

func (s *Server) serveA2UIRendered(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/canvas/a2ui/")
	if s.canvasStore == nil {
		http.Error(w, "canvas not initialized", http.StatusServiceUnavailable)
		return
	}

	entry, ok := s.canvasStore.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if entry.Type != canvas.EntryTypeA2UI {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(entry.Content))
		return
	}

	doc, err := canvas.ParseA2UI(entry.Content)
	if err != nil {
		http.Error(w, "invalid a2ui: "+err.Error(), http.StatusBadRequest)
		return
	}

	renderer := canvas.NewA2UIRenderer()
	html, err := renderer.Render(doc)
	if err != nil {
		http.Error(w, "render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

func (s *Server) handleCanvasStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.canvasStore == nil {
		http.NotFound(w, r)
		return
	}

	filePath := strings.TrimPrefix(r.URL.Path, "/__openclaw__/canvas/")
	if filePath == "" || strings.Contains(filePath, "..") {
		http.NotFound(w, r)
		return
	}

	fullPath := filepath.Join(s.canvasStore.BaseDir(), "entries", filePath)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	baseAbs, _ := filepath.Abs(filepath.Join(s.canvasStore.BaseDir(), "entries"))
	if !strings.HasPrefix(absPath, baseAbs) {
		http.NotFound(w, r)
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	contentType := "application/octet-stream"
	switch ext {
	case ".html":
		contentType = "text/html; charset=utf-8"
	case ".json":
		contentType = "application/json"
	case ".md":
		contentType = "text/markdown"
	case ".txt":
		contentType = "text/plain"
	case ".css":
		contentType = "text/css"
	case ".js":
		contentType = "application/javascript"
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleA2UIStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.canvasStore == nil {
		http.NotFound(w, r)
		return
	}

	filePath := strings.TrimPrefix(r.URL.Path, "/__openclaw__/a2ui/")
	if filePath == "" || strings.Contains(filePath, "..") {
		http.NotFound(w, r)
		return
	}

	fullPath := filepath.Join(s.canvasStore.BaseDir(), "a2ui", filePath)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	baseAbs, _ := filepath.Abs(filepath.Join(s.canvasStore.BaseDir(), "a2ui"))
	if !strings.HasPrefix(absPath, baseAbs) {
		http.NotFound(w, r)
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	contentType := "application/octet-stream"
	switch ext {
	case ".html":
		contentType = "text/html; charset=utf-8"
	case ".json":
		contentType = "application/json"
	case ".css":
		contentType = "text/css"
	case ".js":
		contentType = "application/javascript"
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

const canvasIndexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AnyClaw Canvas</title>
    <style>
        :root {
            --bg: #f4ede2;
            --panel: rgba(255, 251, 245, 0.92);
            --line: rgba(80, 63, 42, 0.14);
            --text: #1c1a18;
            --muted: #6e655a;
            --accent: #0f766e;
            --radius: 18px;
        }
        * { box-sizing: border-box; }
        body {
            margin: 0;
            min-height: 100vh;
            font-family: "Aptos", "Segoe UI Variable", "Segoe UI", sans-serif;
            color: var(--text);
            background: radial-gradient(circle at top left, rgba(15, 118, 110, 0.18), transparent 30%),
                        radial-gradient(circle at bottom right, rgba(180, 83, 9, 0.12), transparent 30%),
                        linear-gradient(180deg, #f7f1e6 0%, #f2e8d9 100%);
        }
        .container { max-width: 1200px; margin: 0 auto; padding: 24px; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px; }
        .header h1 { margin: 0; font-size: 28px; }
        .btn {
            padding: 10px 18px;
            border-radius: 12px;
            border: none;
            background: linear-gradient(135deg, var(--accent), #115e59);
            color: white;
            font-weight: 600;
            cursor: pointer;
        }
        .card {
            background: var(--panel);
            border-radius: var(--radius);
            padding: 18px;
            margin-bottom: 16px;
            border: 1px solid var(--line);
            box-shadow: 0 18px 42px rgba(47, 35, 21, 0.12);
        }
        .card h3 { margin: 0 0 8px; font-size: 17px; }
        .card .meta { color: var(--muted); font-size: 13px; }
        .card .actions { margin-top: 12px; display: flex; gap: 8px; }
        .card .actions a {
            padding: 6px 12px;
            border-radius: 8px;
            background: rgba(15, 118, 110, 0.1);
            color: var(--accent);
            text-decoration: none;
            font-size: 13px;
            font-weight: 600;
        }
        .empty { text-align: center; color: var(--muted); padding: 48px; }
        .type-badge {
            display: inline-block;
            padding: 3px 8px;
            border-radius: 6px;
            font-size: 11px;
            font-weight: 600;
            background: rgba(15, 118, 110, 0.1);
            color: var(--accent);
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>AnyClaw Canvas</h1>
            <button class="btn" onclick="location.reload()">Refresh</button>
        </div>
        <div id="entries"></div>
    </div>
    <script>
        async function loadEntries() {
            try {
                const resp = await fetch('/api/canvas');
                const entries = await resp.json();
                const container = document.getElementById('entries');
                if (!entries.length) {
                    container.innerHTML = '<div class="empty">No canvas entries yet. Agents can push content using canvas_push tool.</div>';
                    return;
                }
                container.innerHTML = entries.map(function(e) {
                    var viewUrl = e.type === 'a2ui' ? '/canvas/a2ui/' + e.id : '/canvas/view/' + e.id;
                    return '<div class="card">' +
                        '<h3>' + escapeHTML(e.name) + ' <span class="type-badge">' + escapeHTML(e.type) + '</span></h3>' +
                        '<div class="meta">v' + e.version + ' | Updated: ' + new Date(e.updated_at).toLocaleString() + '</div>' +
                        '<div class="actions">' +
                        '<a href="' + viewUrl + '">View</a>' +
                        '<a href="/api/canvas/' + e.id + '">JSON</a>' +
                        '<a href="/api/canvas/' + e.id + '/versions">Versions</a>' +
                        '</div></div>';
                }).join('');
            } catch (err) {
                document.getElementById('entries').innerHTML = '<div class="empty">Failed to load entries: ' + escapeHTML(err.message) + '</div>';
            }
        }
        function escapeHTML(str) {
            return String(str || '').replace(/[&<>"']/g, function(c) {
                return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
            });
        }
        loadEntries();
    </script>
</body>
</html>`

func (s *Server) broadcastCanvasUpdate(entry *canvas.CanvasEntry) {
	if s.canvasHub != nil {
		s.canvasHub.Broadcast(entry)
	}
}

func (s *Server) handleCanvasRoute(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/versions") {
		s.handleCanvasVersions(w, r)
		return
	}
	if strings.Contains(path, "/versions/") {
		s.handleCanvasVersionGet(w, r)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(path, "/reset") {
		s.handleCanvasReset(w, r)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(path, "/render") {
		s.handleCanvasRender(w, r)
		return
	}
	if r.Method == http.MethodDelete {
		s.handleCanvasDelete(w, r)
		return
	}
	if r.Method == http.MethodPost {
		s.handleCanvasPush(w, r)
		return
	}
	s.handleCanvasGet(w, r)
}

func (s *Server) handleCanvasWS(w http.ResponseWriter, r *http.Request) {
	if s.canvasHub == nil {
		http.Error(w, "canvas websocket not initialized", http.StatusServiceUnavailable)
		return
	}

	conn, err := openClawWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	s.canvasHub.Register(conn)

	go func() {
		defer s.canvasHub.Unregister(conn)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()
}
