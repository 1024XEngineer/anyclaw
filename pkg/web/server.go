package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type Server struct {
	config      *config.Config
	httpServer  *http.Server
	mux         *http.ServeMux
	controlUI   *ControlUI
	canvas      *CanvasServer
	gatewayAddr string
	version     string
	startedAt   time.Time
}

func NewServer(cfg *config.Config, gatewayAddr, version string) *Server {
	s := &Server{
		config:      cfg,
		mux:         http.NewServeMux(),
		gatewayAddr: gatewayAddr,
		version:     version,
		startedAt:   time.Now(),
	}

	s.controlUI = NewControlUI(cfg, gatewayAddr, version)
	s.canvas = NewCanvasServer(&CanvasConfig{Port: 8081})
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/", s.handleRoot)
	s.mux.Handle("/control", http.RedirectHandler("/control/", http.StatusFound))
	s.mux.Handle("/control/", http.StripPrefix("/control/", s.controlUI))
	s.mux.Handle("/canvas", http.RedirectHandler("/canvas/", http.StatusFound))
	s.mux.Handle("/canvas/", http.StripPrefix("/canvas/", s.canvas))
	s.mux.Handle("/api/", s.handleAPI())
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket") {
		fmt.Fprint(w, "WebSocket endpoint - connect through the configured AnyClaw gateway for live events.")
		return
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AnyClaw</title>
  <style>
    :root {
      --bg: #f4efe8;
      --panel: rgba(255, 251, 245, 0.88);
      --ink: #1c2228;
      --muted: #5f6972;
      --line: rgba(28, 34, 40, 0.1);
      --accent: #cb5d2e;
      --accent-soft: #f2c786;
      --card-shadow: 0 24px 64px rgba(73, 55, 42, 0.16);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      padding: 24px;
      font-family: "Bahnschrift", "Aptos", "Segoe UI", sans-serif;
      color: var(--ink);
      background:
        radial-gradient(circle at 15%% 15%%, rgba(203, 93, 46, 0.22), transparent 24%%),
        radial-gradient(circle at 88%% 10%%, rgba(242, 199, 134, 0.45), transparent 24%%),
        linear-gradient(150deg, #f7f3ea 0%%, #efe7da 45%%, #e4dccf 100%%);
    }
    .shell {
      max-width: 1220px;
      margin: 0 auto;
      display: grid;
      gap: 22px;
    }
    .hero {
      background: linear-gradient(135deg, rgba(255, 250, 244, 0.98), rgba(248, 237, 220, 0.92));
      border-radius: 30px;
      border: 1px solid rgba(255, 255, 255, 0.7);
      box-shadow: var(--card-shadow);
      padding: 30px;
      display: grid;
      gap: 20px;
    }
    .eyebrow {
      display: inline-flex;
      align-items: center;
      gap: 10px;
      letter-spacing: 0.18em;
      text-transform: uppercase;
      font-size: 12px;
      color: var(--muted);
    }
    .eyebrow::before {
      content: "";
      width: 30px;
      height: 2px;
      border-radius: 999px;
      background: linear-gradient(90deg, var(--accent), var(--accent-soft));
    }
    h1 {
      margin: 6px 0 10px;
      font-size: clamp(38px, 8vw, 76px);
      line-height: 0.92;
      letter-spacing: -0.05em;
      max-width: 720px;
    }
    .subtitle {
      margin: 0;
      max-width: 720px;
      color: var(--muted);
      font-size: 17px;
      line-height: 1.7;
    }
    .hero-grid, .cards {
      display: grid;
      grid-template-columns: repeat(12, minmax(0, 1fr));
      gap: 18px;
    }
    .metric, .card {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 22px;
      box-shadow: 0 16px 36px rgba(73, 55, 42, 0.1);
      backdrop-filter: blur(12px);
    }
    .metric {
      grid-column: span 4;
      padding: 18px 20px;
    }
    .metric label {
      display: block;
      color: var(--muted);
      font-size: 12px;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      margin-bottom: 10px;
    }
    .metric strong {
      display: block;
      font-size: 30px;
      margin-bottom: 6px;
      line-height: 1.02;
    }
    .metric span {
      color: var(--muted);
      font-size: 14px;
    }
    .card {
      grid-column: span 6;
      padding: 24px;
      display: grid;
      gap: 14px;
    }
    .card h2 {
      margin: 0;
      font-size: 28px;
      letter-spacing: -0.03em;
    }
    .card p {
      margin: 0;
      color: var(--muted);
      line-height: 1.7;
    }
    .actions {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      margin-top: 6px;
    }
    .button {
      display: inline-flex;
      align-items: center;
      gap: 10px;
      text-decoration: none;
      padding: 12px 18px;
      border-radius: 999px;
      font-weight: 700;
      border: 1px solid rgba(28, 34, 40, 0.08);
      color: var(--ink);
      background: rgba(255, 255, 255, 0.86);
    }
    .button.primary {
      color: white;
      border-color: transparent;
      background: linear-gradient(135deg, var(--accent), #dc7d36);
    }
    .meta {
      margin-top: 8px;
      color: var(--muted);
      font-size: 13px;
    }
    @media (max-width: 920px) {
      body { padding: 16px; }
      .metric, .card { grid-column: 1 / -1; }
    }
  </style>
</head>
<body>
  <main class="shell">
    <section class="hero">
      <div class="eyebrow">AnyClaw Local Surface</div>
      <div>
        <h1>One clean entry for the agent workspace.</h1>
        <p class="subtitle">This landing page gives you fast access to the control plane, canvas surface, and the lightweight API endpoints without the mojibake and placeholder UI that were here before.</p>
      </div>
      <div class="hero-grid">
        <article class="metric">
          <label>Provider</label>
          <strong>%s</strong>
          <span>%s</span>
        </article>
        <article class="metric">
          <label>Gateway</label>
          <strong>%s</strong>
          <span>Current websocket / HTTP entry</span>
        </article>
        <article class="metric">
          <label>Version</label>
          <strong>%s</strong>
          <span>Uptime %s</span>
        </article>
      </div>
    </section>
    <section class="cards">
      <article class="card">
        <h2>Control Center</h2>
        <p>Inspect the runtime snapshot, surface channels, and confirm what tools the local web layer is currently advertising.</p>
        <div class="actions">
          <a class="button primary" href="/control/">Open Control Center</a>
          <a class="button" href="/api/status">Status JSON</a>
        </div>
      </article>
      <article class="card">
        <h2>Canvas Studio</h2>
        <p>Push content, inspect the live canvas state, and watch updates stream in without waiting for a manual refresh.</p>
        <div class="actions">
          <a class="button primary" href="/canvas/">Open Canvas</a>
          <a class="button" href="/api/tools">Tool JSON</a>
        </div>
        <div class="meta">API routes: <code>/api/status</code>, <code>/api/channels</code>, <code>/api/tools</code>, <code>/api/sessions</code></div>
      </article>
    </section>
  </main>
</body>
</html>`,
		s.config.LLM.Provider,
		s.config.LLM.Model,
		s.gatewayAddr,
		s.version,
		time.Since(s.startedAt).Round(time.Second).String(),
	)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

func (s *Server) handleAPI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/")
		w.Header().Set("Content-Type", "application/json")

		switch path {
		case "status":
			s.handleAPIStatus(w, r)
		case "channels":
			s.handleAPIChannels(w, r)
		case "tools":
			s.handleAPITools(w, r)
		case "sessions":
			s.handleAPISessions(w, r)
		case "canvas/push":
			s.handleCanvasPush(w, r)
		case "canvas/reset":
			s.handleCanvasReset(w, r)
		default:
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}
	})
}

func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":   "running",
		"version":  s.version,
		"provider": s.config.LLM.Provider,
		"model":    s.config.LLM.Model,
		"gateway":  s.gatewayAddr,
		"uptime":   time.Since(s.startedAt).Round(time.Second).String(),
	})
}

func (s *Server) handleAPIChannels(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]map[string]any{
		{"name": "telegram", "status": "disconnected", "messages": 0},
		{"name": "slack", "status": "disconnected", "messages": 0},
		{"name": "discord", "status": "disconnected", "messages": 0},
		{"name": "whatsapp", "status": "disconnected", "messages": 0},
		{"name": "signal", "status": "disconnected", "messages": 0},
	})
}

func (s *Server) handleAPITools(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]map[string]any{
		{"name": "read_file", "description": "Read file contents"},
		{"name": "write_file", "description": "Write content to file"},
		{"name": "run_command", "description": "Execute shell command"},
		{"name": "browser_navigate", "description": "Navigate browser to URL"},
		{"name": "web_search", "description": "Search the web"},
		{"name": "canvas_push", "description": "Push content to canvas"},
		{"name": "canvas_eval", "description": "Evaluate JavaScript on canvas"},
	})
}

func (s *Server) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]map[string]any{})
}

func (s *Server) handleCanvasPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Content string `json:"content"`
		Reset   bool   `json:"reset"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"length":  len(req.Content),
		"reset":   req.Reset,
	})
}

func (s *Server) handleCanvasReset(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) Start(addr string) error {
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func decodeJSON(reader io.Reader, v any) error {
	dec := json.NewDecoder(reader)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
