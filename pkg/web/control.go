package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type ControlUI struct {
	config *config.Config
	server *http.Server
	mux    *http.ServeMux

	gatewayAddr string
	version     string
	startedAt   time.Time
}

type StatusData struct {
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Address   string    `json:"address"`
	StartedAt time.Time `json:"started_at"`
	Sessions  int       `json:"sessions"`
	Channels  int       `json:"channels"`
	Skills    int       `json:"skills"`
	Tools     int       `json:"tools"`
	Uptime    string    `json:"uptime"`
}

type controlPageData struct {
	Status      StatusData
	StatusClass string
	Channels    []ChannelInfo
	Tools       []ToolInfo
}

var controlUITemplates = map[string]string{
	"index": `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AnyClaw Control Center</title>
  <style>
    :root {
      --bg: #f4efe6;
      --panel: rgba(255, 251, 245, 0.86);
      --panel-strong: #fffaf2;
      --ink: #182026;
      --muted: #5c6770;
      --line: rgba(24, 32, 38, 0.12);
      --accent: #d45c2d;
      --accent-soft: #f2c88a;
      --good: #1c8c5f;
      --warn: #b56a11;
      --shadow: 0 22px 60px rgba(86, 61, 43, 0.15);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      color: var(--ink);
      font-family: "Bahnschrift", "Aptos", "Segoe UI", sans-serif;
      background:
        radial-gradient(circle at top left, rgba(212, 92, 45, 0.18), transparent 34%),
        radial-gradient(circle at top right, rgba(242, 200, 138, 0.42), transparent 28%),
        linear-gradient(160deg, #f7f1e7 0%, #f2ece2 52%, #e7dfd0 100%);
      padding: 28px;
    }
    .shell {
      max-width: 1220px;
      margin: 0 auto;
      display: grid;
      gap: 22px;
    }
    .hero {
      background: linear-gradient(135deg, rgba(255, 250, 242, 0.96), rgba(249, 237, 214, 0.92));
      border: 1px solid rgba(255, 255, 255, 0.65);
      border-radius: 28px;
      padding: 28px;
      box-shadow: var(--shadow);
      display: grid;
      gap: 18px;
    }
    .hero-top {
      display: flex;
      justify-content: space-between;
      gap: 16px;
      align-items: start;
      flex-wrap: wrap;
    }
    .eyebrow {
      display: inline-flex;
      align-items: center;
      gap: 10px;
      font-size: 12px;
      letter-spacing: 0.18em;
      text-transform: uppercase;
      color: var(--muted);
    }
    .eyebrow::before {
      content: "";
      width: 28px;
      height: 2px;
      border-radius: 999px;
      background: linear-gradient(90deg, var(--accent), var(--accent-soft));
    }
    h1 {
      margin: 8px 0 10px;
      font-size: clamp(32px, 5vw, 56px);
      line-height: 0.95;
      letter-spacing: -0.04em;
    }
    .subtext {
      max-width: 720px;
      margin: 0;
      color: var(--muted);
      font-size: 16px;
      line-height: 1.6;
    }
    .status-pill {
      border-radius: 999px;
      padding: 10px 16px;
      font-size: 13px;
      font-weight: 700;
      letter-spacing: 0.06em;
      text-transform: uppercase;
      border: 1px solid rgba(24, 32, 38, 0.1);
      background: rgba(255, 255, 255, 0.72);
      color: var(--ink);
      align-self: start;
    }
    .status-pill.ok {
      color: var(--good);
      border-color: rgba(28, 140, 95, 0.22);
      background: rgba(28, 140, 95, 0.08);
    }
    .hero-grid,
    .content-grid {
      display: grid;
      grid-template-columns: repeat(12, minmax(0, 1fr));
      gap: 18px;
    }
    .metric,
    .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 22px;
      box-shadow: 0 14px 36px rgba(86, 61, 43, 0.08);
      backdrop-filter: blur(10px);
    }
    .metric {
      grid-column: span 3;
      padding: 18px 20px;
    }
    .metric label {
      display: block;
      font-size: 12px;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: 10px;
    }
    .metric strong {
      display: block;
      font-size: 28px;
      line-height: 1.05;
      margin-bottom: 6px;
    }
    .metric span {
      color: var(--muted);
      font-size: 14px;
    }
    .panel {
      padding: 22px;
    }
    .panel h2 {
      margin: 0 0 6px;
      font-size: 22px;
      letter-spacing: -0.02em;
    }
    .panel p {
      margin: 0 0 18px;
      color: var(--muted);
      line-height: 1.6;
    }
    .panel-wide {
      grid-column: span 7;
    }
    .panel-side {
      grid-column: span 5;
    }
    .rows {
      display: grid;
      gap: 10px;
    }
    .row {
      display: grid;
      grid-template-columns: minmax(0, 1.2fr) auto auto;
      gap: 12px;
      align-items: center;
      padding: 14px 16px;
      border-radius: 16px;
      background: var(--panel-strong);
      border: 1px solid rgba(24, 32, 38, 0.08);
    }
    .row strong { font-size: 15px; }
    .row span, .row code {
      color: var(--muted);
      font-size: 13px;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      min-width: 92px;
      padding: 8px 12px;
      border-radius: 999px;
      font-size: 12px;
      font-weight: 700;
      letter-spacing: 0.06em;
      text-transform: uppercase;
      background: rgba(24, 32, 38, 0.06);
      color: var(--muted);
    }
    .badge.online {
      background: rgba(28, 140, 95, 0.1);
      color: var(--good);
    }
    .badge.offline {
      background: rgba(181, 106, 17, 0.1);
      color: var(--warn);
    }
    .links {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
    }
    .action {
      display: inline-flex;
      align-items: center;
      gap: 10px;
      padding: 12px 16px;
      border-radius: 999px;
      text-decoration: none;
      color: var(--ink);
      background: rgba(255, 255, 255, 0.84);
      border: 1px solid rgba(24, 32, 38, 0.08);
      font-weight: 700;
    }
    .action.primary {
      background: linear-gradient(135deg, var(--accent), #e07d35);
      color: white;
      border-color: transparent;
    }
    .footnote {
      margin-top: 18px;
      font-size: 13px;
      color: var(--muted);
    }
    @media (max-width: 960px) {
      body { padding: 18px; }
      .metric, .panel-wide, .panel-side { grid-column: 1 / -1; }
      .row { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <main class="shell">
    <section class="hero">
      <div class="hero-top">
        <div>
          <div class="eyebrow">AnyClaw Control Center</div>
          <h1>Keep the local agent stack visible.</h1>
          <p class="subtext">A quick view of provider state, local tools, channels, and the routes your AnyClaw workspace exposes right now.</p>
        </div>
        <div class="status-pill {{.StatusClass}}">{{.Status.Status}}</div>
      </div>
      <div class="hero-grid">
        <article class="metric">
          <label>Provider</label>
          <strong>{{.Status.Provider}}</strong>
          <span>{{.Status.Model}}</span>
        </article>
        <article class="metric">
          <label>Gateway</label>
          <strong>{{.Status.Address}}</strong>
          <span>Web and websocket entry</span>
        </article>
        <article class="metric">
          <label>Uptime</label>
          <strong>{{.Status.Uptime}}</strong>
          <span>Started {{.Status.StartedAt.Format "2006-01-02 15:04:05"}}</span>
        </article>
        <article class="metric">
          <label>Surface</label>
          <strong>{{.Status.Tools}}</strong>
          <span>{{.Status.Channels}} channels wired</span>
        </article>
      </div>
    </section>

    <section class="content-grid">
      <article class="panel panel-wide">
        <h2>Channel snapshot</h2>
        <p>These rows are the current control-plane view exposed by the lightweight web UI.</p>
        <div class="rows">
          {{range .Channels}}
          <div class="row">
            <div>
              <strong>{{.Name}}</strong><br>
              <span>Last activity: {{.LastActivity}}</span>
            </div>
            <div class="badge {{if eq .Status "connected"}}online{{else}}offline{{end}}">{{.Status}}</div>
            <code>{{.Messages}} messages</code>
          </div>
          {{end}}
        </div>
      </article>

      <aside class="panel panel-side">
        <h2>Quick routes</h2>
        <p>Jump into the interfaces most people need first, or inspect the raw JSON endpoints directly.</p>
        <div class="links">
          <a class="action primary" href="/canvas/">Open Canvas</a>
          <a class="action" href="/api/status">API Status</a>
          <a class="action" href="/api/channels">API Channels</a>
          <a class="action" href="/api/tools">API Tools</a>
          <a class="action" href="/">Back Home</a>
        </div>
        <p class="footnote">Version {{.Status.Version}}. This page is intentionally lightweight so it can load even when the heavier runtime surfaces are still warming up.</p>
      </aside>
    </section>

    <section class="panel">
      <h2>Available tools</h2>
      <p>Built-in tools currently advertised by the web control layer.</p>
      <div class="rows">
        {{range .Tools}}
        <div class="row">
          <div>
            <strong>{{.Name}}</strong><br>
            <span>{{.Description}}</span>
          </div>
          <div class="badge online">ready</div>
          <code>local tool</code>
        </div>
        {{end}}
      </div>
    </section>
  </main>
</body>
</html>`,
}

func NewControlUI(cfg *config.Config, gatewayAddr, version string) *ControlUI {
	ui := &ControlUI{
		config:      cfg,
		mux:         http.NewServeMux(),
		gatewayAddr: gatewayAddr,
		version:     version,
		startedAt:   time.Now(),
	}

	ui.setupRoutes()
	return ui
}

func (ui *ControlUI) setupRoutes() {
	ui.mux.HandleFunc("/", ui.handleIndex)
	ui.mux.HandleFunc("/status", ui.handleStatus)
	ui.mux.HandleFunc("/api/status", ui.handleAPIStatus)
	ui.mux.HandleFunc("/api/channels", ui.handleAPIChannels)
	ui.mux.HandleFunc("/api/tools", ui.handleAPITools)
	ui.mux.HandleFunc("/api/sessions", ui.handleAPISessions)
	ui.mux.HandleFunc("/ws", ui.handleWebSocket)
}

func (ui *ControlUI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ui.mux.ServeHTTP(w, r)
}

func (ui *ControlUI) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("index").Parse(controlUITemplates["index"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	status := ui.statusSnapshot()
	pageData := controlPageData{
		Status:      status,
		StatusClass: "ok",
		Channels:    ui.sampleChannels(),
		Tools:       ui.sampleTools(),
	}
	if err := tmpl.Execute(w, pageData); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (ui *ControlUI) handleStatus(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusFound)
}

func (ui *ControlUI) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ui.statusSnapshot())
}

func (ui *ControlUI) handleAPIChannels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ui.sampleChannels())
}

func (ui *ControlUI) handleAPITools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ui.sampleTools())
}

func (ui *ControlUI) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]SessionInfo{})
}

func (ui *ControlUI) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, "WebSocket endpoint - use the gateway websocket connection for live control-plane events.")
}

type ChannelInfo struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	Messages     int64  `json:"messages"`
	LastActivity string `json:"last_activity"`
}

type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SessionInfo struct {
	ID        string `json:"id"`
	Agent     string `json:"agent"`
	Channel   string `json:"channel"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
}

func (ui *ControlUI) Start(addr string) error {
	ui.server = &http.Server{
		Addr:         addr,
		Handler:      ui.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return ui.server.ListenAndServe()
}

func (ui *ControlUI) Stop() error {
	if ui.server != nil {
		return ui.server.Close()
	}
	return nil
}

func ServeStatic(dir string) http.Handler {
	return http.FileServer(http.Dir(dir))
}

func ServeSinglePageApp(indexFile string) http.Handler {
	tmpl := template.Must(template.ParseFiles(indexFile))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := os.Stat(path); err == nil {
				http.FileServer(http.Dir(".")).ServeHTTP(w, r)
				return
			}
		}
		_ = tmpl.Execute(w, nil)
	})
}

func (ui *ControlUI) statusSnapshot() StatusData {
	channels := ui.sampleChannels()
	tools := ui.sampleTools()
	return StatusData{
		Status:    "running",
		Version:   ui.version,
		Provider:  ui.config.LLM.Provider,
		Model:     ui.config.LLM.Model,
		Address:   ui.gatewayAddr,
		StartedAt: ui.startedAt,
		Sessions:  0,
		Channels:  len(channels),
		Skills:    0,
		Tools:     len(tools),
		Uptime:    time.Since(ui.startedAt).Round(time.Second).String(),
	}
}

func (ui *ControlUI) sampleChannels() []ChannelInfo {
	return []ChannelInfo{
		{Name: "telegram", Status: "connected", Messages: 0, LastActivity: "waiting for events"},
		{Name: "slack", Status: "disconnected", Messages: 0, LastActivity: "not configured"},
		{Name: "discord", Status: "disconnected", Messages: 0, LastActivity: "not configured"},
	}
}

func (ui *ControlUI) sampleTools() []ToolInfo {
	return []ToolInfo{
		{Name: "read_file", Description: "Read file contents from the local workspace."},
		{Name: "write_file", Description: "Write content back into local files."},
		{Name: "run_command", Description: "Execute shell commands with the configured safety policy."},
		{Name: "browser_navigate", Description: "Navigate and inspect browser pages."},
		{Name: "canvas_push", Description: "Push content into the lightweight canvas surface."},
	}
}
