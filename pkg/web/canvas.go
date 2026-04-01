package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Canvas struct {
	mu          sync.RWMutex
	content     string
	snapshot    string
	history     []CanvasEntry
	clients     map[*websocket.Conn]bool
	wsUpgrader  websocket.Upgrader
	a2uiEnabled bool
}

type CanvasEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
}

func NewCanvas() *Canvas {
	return &Canvas{
		clients: make(map[*websocket.Conn]bool),
		wsUpgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

type CanvasServer struct {
	canvas *Canvas
	mux    *http.ServeMux
	config *CanvasConfig
}

type CanvasConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func NewCanvasServer(cfg *CanvasConfig) *CanvasServer {
	cs := &CanvasServer{
		canvas: NewCanvas(),
		mux:    http.NewServeMux(),
		config: cfg,
	}
	cs.setupRoutes()
	return cs
}

func (cs *CanvasServer) setupRoutes() {
	cs.mux.HandleFunc("/", cs.handleIndex)
	cs.mux.HandleFunc("/ws", cs.handleWebSocket)
	cs.mux.HandleFunc("/api/push", cs.handlePush)
	cs.mux.HandleFunc("/api/reset", cs.handleReset)
	cs.mux.HandleFunc("/api/snapshot", cs.handleSnapshot)
	cs.mux.HandleFunc("/api/eval", cs.handleEval)
	cs.mux.HandleFunc("/api/state", cs.handleState)
	cs.mux.HandleFunc("/api/history", cs.handleHistory)
}

func (cs *CanvasServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cs.mux.ServeHTTP(w, r)
}

func (cs *CanvasServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("canvas").Parse(canvasHTML)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		WSEndpoint string
	}{
		WSEndpoint: "/ws",
	}
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (cs *CanvasServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := cs.canvas.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	cs.canvas.mu.Lock()
	cs.canvas.clients[conn] = true
	currentContent := cs.canvas.content
	currentSnapshot := cs.canvas.snapshot
	historyCount := len(cs.canvas.history)
	cs.canvas.mu.Unlock()

	_ = conn.WriteJSON(map[string]any{
		"type":       "canvas_state",
		"content":    currentContent,
		"snapshot":   currentSnapshot,
		"history":    historyCount,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})

	defer func() {
		cs.canvas.mu.Lock()
		delete(cs.canvas.clients, conn)
		cs.canvas.mu.Unlock()
		_ = conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
		cs.canvas.mu.RLock()
		content := cs.canvas.content
		snapshot := cs.canvas.snapshot
		history := len(cs.canvas.history)
		cs.canvas.mu.RUnlock()

		_ = conn.WriteJSON(map[string]any{
			"type":       "canvas_state",
			"content":    content,
			"snapshot":   snapshot,
			"history":    history,
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func (cs *CanvasServer) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Content string `json:"content"`
		Reset   bool   `json:"reset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	cs.canvas.mu.Lock()
	if req.Reset {
		cs.canvas.content = ""
		cs.canvas.history = nil
	}
	cs.canvas.content += req.Content
	cs.canvas.snapshot = ""
	cs.canvas.history = append(cs.canvas.history, CanvasEntry{
		Timestamp: time.Now().UTC(),
		Type:      "push",
		Content:   req.Content,
	})
	fullContent := cs.canvas.content
	historyCount := len(cs.canvas.history)
	cs.canvas.mu.Unlock()

	cs.canvas.broadcast(map[string]any{
		"type":       "canvas_push",
		"content":    fullContent,
		"delta":      req.Content,
		"reset":      req.Reset,
		"history":    historyCount,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"length":  len(fullContent),
		"history": historyCount,
		"content": fullContent,
	})
}

func (cs *CanvasServer) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cs.canvas.mu.Lock()
	cs.canvas.content = ""
	cs.canvas.snapshot = ""
	cs.canvas.history = nil
	cs.canvas.mu.Unlock()

	cs.canvas.broadcast(map[string]any{
		"type":       "canvas_reset",
		"content":    "",
		"history":    0,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})

	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (cs *CanvasServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cs.canvas.mu.Lock()
	if cs.canvas.snapshot == "" {
		cs.canvas.snapshot = generateSnapshot(cs.canvas.content)
	}
	snapshot := cs.canvas.snapshot
	content := cs.canvas.content
	historyCount := len(cs.canvas.history)
	cs.canvas.mu.Unlock()

	_ = json.NewEncoder(w).Encode(map[string]any{
		"snapshot": snapshot,
		"content":  content,
		"history":  historyCount,
	})
}

func (cs *CanvasServer) handleEval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	result := evalJavaScript(req.Code)
	cs.canvas.broadcast(map[string]any{
		"type":       "canvas_eval",
		"code":       req.Code,
		"result":     result,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})

	_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
}

func (cs *CanvasServer) handleState(w http.ResponseWriter, r *http.Request) {
	cs.canvas.mu.RLock()
	content := cs.canvas.content
	snapshot := cs.canvas.snapshot
	historyCount := len(cs.canvas.history)
	cs.canvas.mu.RUnlock()

	_ = json.NewEncoder(w).Encode(map[string]any{
		"content":  content,
		"snapshot": snapshot,
		"history":  historyCount,
	})
}

func (cs *CanvasServer) handleHistory(w http.ResponseWriter, r *http.Request) {
	cs.canvas.mu.RLock()
	history := make([]CanvasEntry, len(cs.canvas.history))
	copy(history, cs.canvas.history)
	cs.canvas.mu.RUnlock()
	_ = json.NewEncoder(w).Encode(history)
}

func (c *Canvas) broadcast(msg map[string]any) {
	c.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(c.clients))
	for conn := range c.clients {
		clients = append(clients, conn)
	}
	c.mu.RUnlock()

	for _, conn := range clients {
		if err := conn.WriteJSON(msg); err != nil {
			_ = conn.Close()
			c.mu.Lock()
			delete(c.clients, conn)
			c.mu.Unlock()
		}
	}
}

func generateSnapshot(content string) string {
	return base64.StdEncoding.EncodeToString([]byte(content))
}

func evalJavaScript(code string) string {
	return fmt.Sprintf("evaluated: %s", code)
}

var canvasHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AnyClaw Canvas</title>
  <style>
    :root {
      --bg: #f6f0e5;
      --panel: rgba(255, 250, 244, 0.88);
      --panel-strong: rgba(255, 255, 255, 0.9);
      --ink: #192126;
      --muted: #5f6972;
      --line: rgba(25, 33, 38, 0.12);
      --accent: #cf5b2f;
      --accent-soft: #f3c97f;
      --good: #1f8b63;
      --warn: #c46a17;
      --shadow: 0 24px 64px rgba(77, 58, 44, 0.16);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      font-family: "Bahnschrift", "Aptos", "Segoe UI", sans-serif;
      color: var(--ink);
      background:
        radial-gradient(circle at 12% 12%, rgba(207, 91, 47, 0.2), transparent 22%),
        radial-gradient(circle at 88% 8%, rgba(243, 201, 127, 0.4), transparent 25%),
        linear-gradient(155deg, #f9f4ea 0%, #eee5d8 48%, #e3dacd 100%);
      padding: 18px;
    }
    .shell {
      max-width: 1360px;
      margin: 0 auto;
      display: grid;
      gap: 18px;
    }
    .hero {
      display: flex;
      justify-content: space-between;
      gap: 16px;
      flex-wrap: wrap;
      padding: 24px 26px;
      border-radius: 28px;
      background: linear-gradient(135deg, rgba(255, 251, 245, 0.96), rgba(248, 236, 216, 0.92));
      border: 1px solid rgba(255, 255, 255, 0.7);
      box-shadow: var(--shadow);
    }
    .hero h1 {
      margin: 6px 0 8px;
      font-size: clamp(32px, 5vw, 52px);
      line-height: 0.95;
      letter-spacing: -0.04em;
    }
    .hero p {
      margin: 0;
      max-width: 660px;
      color: var(--muted);
      line-height: 1.7;
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
    .status {
      display: grid;
      gap: 10px;
      align-content: start;
      min-width: 240px;
    }
    .status-card, .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 22px;
      box-shadow: 0 14px 36px rgba(77, 58, 44, 0.1);
      backdrop-filter: blur(12px);
    }
    .status-card {
      padding: 16px 18px;
    }
    .status-card label {
      display: block;
      font-size: 12px;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: 8px;
    }
    .status-card strong {
      font-size: 24px;
    }
    .grid {
      display: grid;
      grid-template-columns: 340px minmax(0, 1fr) 320px;
      gap: 18px;
      align-items: start;
    }
    .panel {
      padding: 20px;
    }
    .panel h2 {
      margin: 0 0 6px;
      font-size: 24px;
      letter-spacing: -0.02em;
    }
    .panel p {
      margin: 0 0 16px;
      color: var(--muted);
      line-height: 1.6;
    }
    .stack {
      display: grid;
      gap: 14px;
    }
    textarea,
    input {
      width: 100%;
      border: 1px solid rgba(25, 33, 38, 0.12);
      border-radius: 16px;
      padding: 14px 16px;
      font: inherit;
      color: var(--ink);
      background: var(--panel-strong);
    }
    textarea {
      min-height: 210px;
      resize: vertical;
      line-height: 1.6;
    }
    textarea:focus,
    input:focus {
      outline: none;
      border-color: rgba(207, 91, 47, 0.45);
      box-shadow: 0 0 0 4px rgba(207, 91, 47, 0.12);
    }
    .button-row {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
    }
    button {
      border: none;
      border-radius: 999px;
      padding: 12px 16px;
      font: inherit;
      font-weight: 700;
      cursor: pointer;
      color: var(--ink);
      background: rgba(255, 255, 255, 0.84);
      border: 1px solid rgba(25, 33, 38, 0.08);
    }
    button.primary {
      color: white;
      border-color: transparent;
      background: linear-gradient(135deg, var(--accent), #db7a37);
    }
    button.warn {
      color: #7b3f06;
      background: rgba(243, 201, 127, 0.28);
    }
    .canvas-view {
      min-height: 520px;
      border-radius: 22px;
      padding: 24px;
      background:
        linear-gradient(180deg, rgba(255, 253, 249, 0.95), rgba(251, 246, 236, 0.9)),
        repeating-linear-gradient(
          0deg,
          rgba(25, 33, 38, 0.02),
          rgba(25, 33, 38, 0.02) 30px,
          transparent 30px,
          transparent 60px
        );
      border: 1px solid rgba(25, 33, 38, 0.08);
      white-space: pre-wrap;
      word-break: break-word;
      line-height: 1.7;
      font-family: "Cascadia Code", "Consolas", monospace;
    }
    .log {
      max-height: 520px;
      overflow: auto;
      display: grid;
      gap: 10px;
    }
    .entry {
      border-radius: 16px;
      padding: 12px 14px;
      background: var(--panel-strong);
      border: 1px solid rgba(25, 33, 38, 0.08);
      font-size: 13px;
      line-height: 1.5;
      color: var(--muted);
    }
    .entry strong {
      display: block;
      color: var(--ink);
      margin-bottom: 4px;
    }
    .pill {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 8px 12px;
      border-radius: 999px;
      font-size: 12px;
      font-weight: 700;
      letter-spacing: 0.06em;
      text-transform: uppercase;
      background: rgba(31, 139, 99, 0.1);
      color: var(--good);
    }
    .meta {
      color: var(--muted);
      font-size: 13px;
    }
    @media (max-width: 1120px) {
      .grid { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <main class="shell">
    <section class="hero">
      <div>
        <div class="eyebrow">AnyClaw Canvas Studio</div>
        <h1>Push, inspect, and iterate in one place.</h1>
        <p>The canvas now updates immediately on websocket connect, keeps its state consistent after pushes, and gives you a clearer editing surface instead of prompt-based controls.</p>
      </div>
      <div class="status">
        <div class="status-card">
          <label>Connection</label>
          <strong id="connection-state">Connecting</strong>
        </div>
        <div class="status-card">
          <label>Content length</label>
          <strong id="content-length">0</strong>
        </div>
      </div>
    </section>

    <section class="grid">
      <aside class="panel">
        <h2>Composer</h2>
        <p>Draft new content here, then append it to the live canvas or replace the canvas completely.</p>
        <div class="stack">
          <textarea id="push-input" placeholder="Write markdown, notes, code, or any transient working content here."></textarea>
          <div class="button-row">
            <button class="primary" onclick="pushContent(false)">Append</button>
            <button onclick="pushContent(true)">Replace</button>
            <button class="warn" onclick="resetCanvas()">Reset</button>
          </div>
          <input id="code-input" placeholder="Quick eval note, e.g. console.log('hello')" onkeypress="handleEval(event)">
          <div class="button-row">
            <button onclick="evalCode()">Run Eval</button>
            <button onclick="takeSnapshot()">Snapshot</button>
          </div>
        </div>
      </aside>

      <section class="panel">
        <div class="button-row" style="justify-content: space-between; margin-bottom: 16px;">
          <div class="pill" id="history-pill">History 0</div>
          <div class="meta" id="snapshot-meta">Snapshot ready when requested</div>
        </div>
        <div id="canvas" class="canvas-view"></div>
      </section>

      <aside class="panel">
        <h2>Activity</h2>
        <p>Live events and API responses land here, so you can see what changed without refreshing.</p>
        <div id="log" class="log"></div>
      </aside>
    </section>
  </main>

  <script>
    let ws = null;

    function setConnection(text, ok) {
      const node = document.getElementById('connection-state');
      node.textContent = text;
      node.style.color = ok ? 'var(--good)' : 'var(--warn)';
    }

    function updateCanvasState(data) {
      const content = data.content || '';
      document.getElementById('canvas').textContent = content;
      document.getElementById('content-length').textContent = String(content.length);
      document.getElementById('history-pill').textContent = 'History ' + String(data.history || 0);
      if (data.snapshot) {
        document.getElementById('snapshot-meta').textContent = 'Snapshot size ' + data.snapshot.length;
      }
    }

    function log(title, message) {
      const node = document.createElement('div');
      node.className = 'entry';
      node.innerHTML = '<strong>' + title + '</strong>' + message;
      const target = document.getElementById('log');
      target.prepend(node);
    }

    function connect() {
      const scheme = location.protocol === 'https:' ? 'wss://' : 'ws://';
      ws = new WebSocket(scheme + location.host + '{{.WSEndpoint}}');
      setConnection('Connecting', false);

      ws.onopen = function() {
        setConnection('Connected', true);
        log('Socket', 'Canvas websocket connected.');
      };

      ws.onmessage = function(event) {
        const data = JSON.parse(event.data);
        if (data.type === 'canvas_state' || data.type === 'canvas_push' || data.type === 'canvas_reset') {
          updateCanvasState(data);
          log('Canvas', 'State updated at ' + (data.updated_at || new Date().toISOString()) + '.');
          return;
        }
        if (data.type === 'canvas_eval') {
          log('Eval', 'Result: ' + data.result);
        }
      };

      ws.onclose = function() {
        setConnection('Reconnecting', false);
        log('Socket', 'Connection closed. Retrying in 3s.');
        setTimeout(connect, 3000);
      };
    }

    async function requestJSON(url, options) {
      const response = await fetch(url, options);
      return response.json();
    }

    async function pushContent(reset) {
      const content = document.getElementById('push-input').value;
      const payload = await requestJSON('/api/push', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({content, reset})
      });
      updateCanvasState(payload);
      log(reset ? 'Canvas replace' : 'Canvas append', 'Length now ' + payload.length + '.');
    }

    async function resetCanvas() {
      if (!confirm('Reset the canvas and clear its history?')) {
        return;
      }
      await requestJSON('/api/reset', {method: 'POST'});
      updateCanvasState({content: '', history: 0, snapshot: ''});
      document.getElementById('snapshot-meta').textContent = 'Snapshot cleared';
      log('Canvas reset', 'Canvas cleared.');
    }

    async function takeSnapshot() {
      const payload = await requestJSON('/api/snapshot');
      updateCanvasState(payload);
      document.getElementById('snapshot-meta').textContent = 'Snapshot size ' + payload.snapshot.length;
      log('Snapshot', 'Generated snapshot with ' + payload.snapshot.length + ' characters.');
    }

    async function evalCode() {
      const code = document.getElementById('code-input').value.trim();
      if (!code) {
        return;
      }
      const payload = await requestJSON('/api/eval', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({code})
      });
      document.getElementById('code-input').value = '';
      log('Eval', payload.result);
    }

    function handleEval(event) {
      if (event.key === 'Enter') {
        evalCode();
      }
    }

    connect();
  </script>
</body>
</html>`

type CanvasHandler struct {
	canvas *Canvas
}

func NewCanvasHandler() *CanvasHandler {
	return &CanvasHandler{canvas: NewCanvas()}
}

func (h *CanvasHandler) Push(ctx context.Context, content string, reset bool) error {
	h.canvas.mu.Lock()
	if reset {
		h.canvas.content = ""
		h.canvas.history = nil
	}
	h.canvas.content += content
	h.canvas.snapshot = ""
	h.canvas.history = append(h.canvas.history, CanvasEntry{
		Timestamp: time.Now().UTC(),
		Type:      "push",
		Content:   content,
	})
	fullContent := h.canvas.content
	historyCount := len(h.canvas.history)
	h.canvas.mu.Unlock()

	h.canvas.broadcast(map[string]any{
		"type":       "canvas_push",
		"content":    fullContent,
		"delta":      content,
		"reset":      reset,
		"history":    historyCount,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})
	return nil
}

func (h *CanvasHandler) Reset() error {
	h.canvas.mu.Lock()
	h.canvas.content = ""
	h.canvas.snapshot = ""
	h.canvas.history = nil
	h.canvas.mu.Unlock()

	h.canvas.broadcast(map[string]any{
		"type":       "canvas_reset",
		"content":    "",
		"history":    0,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})
	return nil
}

func (h *CanvasHandler) GetState() (string, string) {
	h.canvas.mu.RLock()
	defer h.canvas.mu.RUnlock()
	return h.canvas.content, h.canvas.snapshot
}

func (h *CanvasHandler) GetHistory() []CanvasEntry {
	h.canvas.mu.RLock()
	defer h.canvas.mu.RUnlock()
	history := make([]CanvasEntry, len(h.canvas.history))
	copy(history, h.canvas.history)
	return history
}
