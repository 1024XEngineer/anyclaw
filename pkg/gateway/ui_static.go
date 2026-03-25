package gateway

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed webui/*
var embeddedWebUI embed.FS

func (s *Server) handleRootUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/app", http.StatusTemporaryRedirect)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/app") {
		http.NotFound(w, r)
		return
	}
	sub, err := fs.Sub(embeddedWebUI, "webui")
	if err != nil {
		http.Error(w, "ui unavailable", http.StatusInternalServerError)
		return
	}
	target := strings.TrimPrefix(r.URL.Path, "/app")
	if target == "" || target == "/" {
		serveEmbeddedUIFile(w, r, sub, "index.html")
		return
	}
	cleaned := strings.TrimPrefix(path.Clean(target), "/")
	if cleaned == "." || cleaned == "" {
		serveEmbeddedUIFile(w, r, sub, "index.html")
		return
	}
	if !strings.Contains(path.Base(cleaned), ".") {
		serveEmbeddedUIFile(w, r, sub, "index.html")
		return
	}
	serveEmbeddedUIFile(w, r, sub, cleaned)
}

func serveEmbeddedUIFile(w http.ResponseWriter, r *http.Request, files fs.FS, name string) {
	data, err := fs.ReadFile(files, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}
