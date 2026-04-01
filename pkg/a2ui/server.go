package a2ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"
)

var HTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AnyClaw A2UI</title>
    <style>
        :root {
            --bg-primary: #0d1117;
            --bg-secondary: #161b22;
            --bg-tertiary: #21262d;
            --border: #30363d;
            --text-primary: #c9d1d9;
            --text-secondary: #8b949e;
            --accent: #58a6ff;
            --success: #3fb950;
            --warning: #d29922;
            --error: #f85149;
            --purple: #a371f7;
        }
        
        * { margin: 0; padding: 0; box-sizing: border-box; }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            height: 100vh;
            overflow: hidden;
        }
        
        .app {
            display: flex;
            flex-direction: column;
            height: 100vh;
        }
        
        header {
            background: var(--bg-secondary);
            border-bottom: 1px solid var(--border);
            padding: 12px 20px;
            display: flex;
            align-items: center;
            justify-content: space-between;
        }
        
        .logo {
            display: flex;
            align-items: center;
            gap: 12px;
        }
        
        .logo h1 {
            font-size: 18px;
            color: var(--accent);
        }
        
        .badge {
            background: var(--purple);
            color: white;
            padding: 2px 8px;
            border-radius: 4px;
            font-size: 11px;
            font-weight: 600;
        }
        
        .status {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 12px;
            color: var(--text-secondary);
        }
        
        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: var(--error);
        }
        
        .status-dot.connected {
            background: var(--success);
        }
        
        .main {
            display: flex;
            flex: 1;
            overflow: hidden;
        }
        
        .sidebar {
            width: 240px;
            background: var(--bg-secondary);
            border-right: 1px solid var(--border);
            display: flex;
            flex-direction: column;
        }
        
        .sidebar-section {
            padding: 12px;
            border-bottom: 1px solid var(--border);
        }
        
        .sidebar-title {
            font-size: 11px;
            text-transform: uppercase;
            color: var(--text-secondary);
            margin-bottom: 8px;
        }
        
        .tool-list {
            display: flex;
            flex-direction: column;
            gap: 4px;
        }
        
        .tool-item {
            padding: 8px;
            background: var(--bg-tertiary);
            border-radius: 6px;
            cursor: pointer;
            font-size: 12px;
            transition: background 0.2s;
        }
        
        .tool-item:hover {
            background: var(--border);
        }
        
        .content {
            flex: 1;
            display: flex;
            flex-direction: column;
        }
        
        .chat-container {
            flex: 1;
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }
        
        .messages {
            flex: 1;
            overflow-y: auto;
            padding: 20px;
            display: flex;
            flex-direction: column;
            gap: 12px;
        }
        
        .message {
            max-width: 80%;
            padding: 12px 16px;
            border-radius: 12px;
            line-height: 1.5;
            white-space: pre-wrap;
        }
        
        .message.user {
            align-self: flex-end;
            background: var(--accent);
            color: white;
            border-bottom-right-radius: 4px;
        }
        
        .message.assistant {
            align-self: flex-start;
            background: var(--bg-tertiary);
            border-bottom-left-radius: 4px;
        }
        
        .message.thinking {
            color: var(--text-secondary);
            font-style: italic;
        }
        
        .message.tool {
            align-self: flex-start;
            background: var(--bg-secondary);
            border: 1px solid var(--border);
            font-family: monospace;
            font-size: 12px;
        }
        
        .tool-call {
            background: var(--bg-primary);
            border-left: 3px solid var(--purple);
            padding: 8px 12px;
            margin: 8px 0;
            font-family: monospace;
            font-size: 12px;
        }
        
        .tool-call-name {
            color: var(--purple);
            font-weight: 600;
        }
        
        .input-area {
            padding: 16px;
            background: var(--bg-secondary);
            border-top: 1px solid var(--border);
            display: flex;
            gap: 12px;
        }
        
        .input-area input {
            flex: 1;
            background: var(--bg-primary);
            border: 1px solid var(--border);
            border-radius: 8px;
            padding: 12px 16px;
            color: var(--text-primary);
            font-size: 14px;
            outline: none;
        }
        
        .input-area input:focus {
            border-color: var(--accent);
        }
        
        .input-area button {
            background: var(--accent);
            color: white;
            border: none;
            padding: 12px 24px;
            border-radius: 8px;
            cursor: pointer;
            font-weight: 600;
        }
        
        .input-area button:hover {
            opacity: 0.9;
        }
        
        .input-area button:disabled {
            background: var(--border);
            cursor: not-allowed;
        }
        
        .canvas-panel {
            display: none;
            flex: 1;
            flex-direction: column;
        }
        
        .canvas-panel.active {
            display: flex;
        }
        
        .canvas-editor {
            flex: 1;
            background: var(--bg-primary);
            padding: 20px;
            font-family: 'SF Mono', Monaco, monospace;
            font-size: 14px;
            color: var(--text-primary);
            border: none;
            resize: none;
            outline: none;
        }
        
        .tabs {
            display: flex;
            gap: 4px;
            padding: 8px 16px;
            background: var(--bg-secondary);
            border-bottom: 1px solid var(--border);
        }
        
        .tab {
            padding: 8px 16px;
            background: transparent;
            color: var(--text-secondary);
            border: none;
            border-radius: 6px;
            cursor: pointer;
            font-size: 13px;
        }
        
        .tab:hover {
            background: var(--bg-tertiary);
        }
        
        .tab.active {
            background: var(--accent);
            color: white;
        }
        
        .approval-modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(0,0,0,0.7);
            align-items: center;
            justify-content: center;
            z-index: 1000;
        }
        
        .approval-modal.active {
            display: flex;
        }
        
        .approval-content {
            background: var(--bg-secondary);
            border-radius: 12px;
            padding: 24px;
            max-width: 500px;
            width: 90%;
        }
        
        .approval-title {
            font-size: 18px;
            margin-bottom: 16px;
            color: var(--warning);
        }
        
        .approval-details {
            background: var(--bg-primary);
            padding: 12px;
            border-radius: 8px;
            font-family: monospace;
            font-size: 12px;
            margin-bottom: 16px;
            max-height: 200px;
            overflow-y: auto;
        }
        
        .approval-actions {
            display: flex;
            gap: 12px;
            justify-content: flex-end;
        }
        
        .btn {
            padding: 10px 20px;
            border-radius: 6px;
            border: none;
            cursor: pointer;
            font-weight: 600;
        }
        
        .btn-approve {
            background: var(--success);
            color: white;
        }
        
        .btn-deny {
            background: var(--error);
            color: white;
        }
        
        .progress-bar {
            height: 4px;
            background: var(--bg-tertiary);
            width: 100%;
            position: fixed;
            top: 0;
            left: 0;
        }
        
        .progress-fill {
            height: 100%;
            background: var(--accent);
            width: 0;
            transition: width 0.3s;
        }
        
        .hidden { display: none !important; }
    </style>
</head>
<body>
    <div class="app">
        <div class="progress-bar"><div class="progress-fill" id="progress"></div></div>
        
        <header>
            <div class="logo">
                <h1>AnyClaw</h1>
                <span class="badge">A2UI</span>
            </div>
            <div class="status">
                <div class="status-dot" id="status-dot"></div>
                <span id="status-text">Connecting...</span>
            </div>
        </header>
        
        <div class="main">
            <aside class="sidebar">
                <div class="sidebar-section">
                    <div class="sidebar-title">Tools</div>
                    <div class="tool-list">
                        <div class="tool-item" onclick="insertTool('read_file')">read_file</div>
                        <div class="tool-item" onclick="insertTool('write_file')">write_file</div>
                        <div class="tool-item" onclick="insertTool('run_command')">run_command</div>
                        <div class="tool-item" onclick="insertTool('browser_navigate')">browser_navigate</div>
                        <div class="tool-item" onclick="insertTool('search_files')">search_files</div>
                    </div>
                </div>
                <div class="sidebar-section">
                    <div class="sidebar-title">Session</div>
                    <div class="tool-list">
                        <div class="tool-item" onclick="clearHistory()">Clear History</div>
                        <div class="tool-item" onclick="exportSession()">Export</div>
                    </div>
                </div>
            </aside>
            
            <div class="content">
                <div class="tabs">
                    <button class="tab active" onclick="switchTab('chat')">Chat</button>
                    <button class="tab" onclick="switchTab('canvas')">Canvas</button>
                </div>
                
                <div class="chat-container" id="chat-panel">
                    <div class="messages" id="messages"></div>
                    <div class="input-area">
                        <input type="text" id="message-input" placeholder="Type a message..." onkeypress="handleKeypress(event)">
                        <button id="send-btn" onclick="sendMessage()">Send</button>
                    </div>
                </div>
                
                <div class="canvas-panel" id="canvas-panel">
                    <textarea class="canvas-editor" id="canvas-editor" placeholder="Canvas content..."></textarea>
                    <div class="input-area">
                        <button onclick="saveCanvas()">Save</button>
                        <button onclick="clearCanvas()">Clear</button>
                    </div>
                </div>
            </div>
        </div>
    </div>
    
    <div class="approval-modal" id="approval-modal">
        <div class="approval-content">
            <div class="approval-title">⚠️ Approval Required</div>
            <div class="approval-details" id="approval-details"></div>
            <div class="approval-actions">
                <button class="btn btn-deny" onclick="denyApproval()">Deny</button>
                <button class="btn btn-approve" onclick="approveApproval()">Approve</button>
            </div>
        </div>
    </div>

    <script>
        let ws = null;
        let sessionId = '';
        let currentApprovalId = null;
        let pendingApproval = null;
        
        function connect() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            ws = new WebSocket(protocol + '//' + window.location.host + '/api/a2ui/ws');
            
            ws.onopen = () => {
                document.getElementById('status-dot').classList.add('connected');
                document.getElementById('status-text').textContent = 'Connected';
                
                ws.send(JSON.stringify({
                    type: 'join',
                    payload: { session_id: sessionId }
                }));
            };
            
            ws.onmessage = (event) => {
                const msg = JSON.parse(event.data);
                handleMessage(msg);
            };
            
            ws.onclose = () => {
                document.getElementById('status-dot').classList.remove('connected');
                document.getElementById('status-text').textContent = 'Disconnected';
                setTimeout(connect, 3000);
            };
            
            ws.onerror = (err) => {
                console.error('WS Error:', err);
            };
        }
        
        function handleMessage(msg) {
            switch(msg.type) {
                case 'joined':
                    sessionId = msg.payload.session_id;
                    break;
                    
                case 'thinking':
                    if (msg.payload.thinking) {
                        addMessage('assistant', 'Thinking...', 'thinking');
                    }
                    break;
                    
                case 'chat_response':
                case 'complete':
                    clearThinking();
                    if (msg.payload.response) {
                        addMessage('assistant', msg.payload.response);
                    }
                    break;
                    
                case 'tool_response':
                    clearThinking();
                    if (msg.payload.result) {
                        addMessage('tool', msg.payload.result);
                    }
                    break;
                    
                case 'error':
                    clearThinking();
                    addMessage('assistant', 'Error: ' + msg.payload.message, 'error');
                    break;
                    
                case 'approval_required':
                    showApprovalModal(msg.payload);
                    break;
            }
        }
        
        function sendMessage() {
            const input = document.getElementById('message-input');
            const message = input.value.trim();
            if (!message) return;
            
            addMessage('user', message);
            input.value = '';
            
            showProgress();
            
            ws.send(JSON.stringify({
                type: 'chat',
                payload: { message: message }
            }));
        }
        
        function handleKeypress(event) {
            if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault();
                sendMessage();
            }
        }
        
        function addMessage(role, content, extraClass = '') {
            const messages = document.getElementById('messages');
            const div = document.createElement('div');
            div.className = 'message ' + role + ' ' + extraClass;
            
            if (role === 'tool') {
                try {
                    const data = JSON.parse(content);
                    div.innerHTML = '<div class="tool-call"><span class="tool-call-name">' + 
                        (data.tool || 'tool') + '</span><pre>' + 
                        JSON.stringify(data.input || data, null, 2) + '</pre></div>';
                } catch {
                    div.textContent = content;
                }
            } else {
                div.textContent = content;
            }
            
            messages.appendChild(div);
            messages.scrollTop = messages.scrollHeight;
        }
        
        function clearThinking() {
            const thinking = document.querySelector('.message.thinking');
            if (thinking) thinking.remove();
            hideProgress();
        }
        
        function showApprovalModal(data) {
            pendingApproval = data;
            document.getElementById('approval-details').textContent = JSON.stringify(data, null, 2);
            document.getElementById('approval-modal').classList.add('active');
        }
        
        function approveApproval() {
            if (!pendingApproval) return;
            
            ws.send(JSON.stringify({
                type: 'approval',
                payload: {
                    approval_id: pendingApproval.id,
                    approved: true
                }
            }));
            
            document.getElementById('approval-modal').classList.remove('active');
            pendingApproval = null;
        }
        
        function denyApproval() {
            if (!pendingApproval) return;
            
            ws.send(JSON.stringify({
                type: 'approval',
                payload: {
                    approval_id: pendingApproval.id,
                    approved: false,
                    comment: 'Denied by user'
                }
            }));
            
            document.getElementById('approval-modal').classList.remove('active');
            pendingApproval = null;
        }
        
        function switchTab(tab) {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.chat-container, .canvas-panel').forEach(p => p.classList.remove('active'));
            
            event.target.classList.add('active');
            
            if (tab === 'chat') {
                document.getElementById('chat-panel').classList.add('active');
            } else {
                document.getElementById('canvas-panel').classList.add('active');
            }
        }
        
        function insertTool(toolName) {
            const input = document.getElementById('message-input');
            input.value = JSON.stringify({ tool: toolName, input: {} }, null, 2);
            input.focus();
        }
        
        function clearHistory() {
            document.getElementById('messages').innerHTML = '';
        }
        
        function exportSession() {
            const messages = document.getElementById('messages').innerText;
            const blob = new Blob([messages], { type: 'text/plain' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = 'session-' + sessionId + '.txt';
            a.click();
        }
        
        function saveCanvas() {
            const content = document.getElementById('canvas-editor').value;
            ws.send(JSON.stringify({
                type: 'canvas',
                payload: {
                    operation: 'set',
                    content: content
                }
            }));
        }
        
        function clearCanvas() {
            document.getElementById('canvas-editor').value = '';
            saveCanvas();
        }
        
        function showProgress() {
            document.getElementById('progress').style.width = '100%';
        }
        
        function hideProgress() {
            document.getElementById('progress').style.width = '0';
        }
        
        connect();
    </script>
</body>
</html>`

var HTML = template.Must(template.New("a2ui").Parse(HTMLTemplate))

func RenderHTML() (string, error) {
	buf := &bytes.Buffer{}
	if err := HTML.Execute(buf, nil); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type Server struct {
	Protocol   *ProtocolHandler
	WSHandler  *WSHandler
	httpServer interface {
		ListenAndServe() error
	}
}

func NewServer(addr string) *Server {
	protocol := NewProtocolHandler()
	wsHandler := NewWSHandler(protocol)

	protocol.EventCB = func(sessionID string, eventType string, data interface{}) {
		wsHandler.SendToSession(sessionID, eventType, data)
	}

	protocol.SetCanvasStore(NewInMemoryCanvas())
	protocol.SetApprovalProvider(NewSimpleApprovalProvider(true))

	return &Server{
		Protocol:  protocol,
		WSHandler: wsHandler,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/a2ui/ws", func(w http.ResponseWriter, r *http.Request) {
		s.WSHandler.HandleHTTP(w, r)
	})

	mux.HandleFunc("/api/a2ui/chat", s.handleChat)
	mux.HandleFunc("/api/a2ui/session", s.handleSession)
	mux.HandleFunc("/api/a2ui/canvas", s.handleCanvasAPI)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html")
			html, _ := RenderHTML()
			fmt.Fprint(w, html)
		}
	})

	return mux
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(errorResponse("", "invalid_request", err.Error()))
		return
	}

	req.Type = MsgTypeRequest
	req.Method = "chat"
	req.Timestamp = Now()

	resp, _ := s.Protocol.HandleRequest(r.Context(), &req)
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(errorResponse("", "invalid_request", err.Error()))
		return
	}

	req.Type = MsgTypeRequest
	req.Method = "session"
	req.Timestamp = Now()

	resp, _ := s.Protocol.HandleRequest(r.Context(), &req)
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCanvasAPI(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(errorResponse("", "invalid_request", err.Error()))
		return
	}

	req.Type = MsgTypeRequest
	req.Method = "canvas"
	req.Timestamp = Now()

	resp, _ := s.Protocol.HandleRequest(r.Context(), &req)
	json.NewEncoder(w).Encode(resp)
}

func Now() int64 {
	return time.Now().Unix()
}

type HTTPServer struct {
	addr   string
	server *http.Server
}

func (s *HTTPServer) ListenAndServe() error {
	return s.server.ListenAndServe()
}

func init() {
	HTML = template.Must(template.New("a2ui").Parse(HTMLTemplate))
}
