package gateway

import "net/http"

const webChatHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AnyClaw WebChat</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0d1117; color: #c9d1d9; height: 100vh; display: flex; flex-direction: column; }
        .header { background: #161b22; padding: 12px 20px; border-bottom: 1px solid #30363d; display: flex; align-items: center; gap: 12px; }
        .header h1 { font-size: 18px; color: #58a6ff; }
        .header .status { font-size: 12px; color: #8b949e; }
        .header .model { background: #21262d; padding: 4px 8px; border-radius: 4px; font-size: 12px; color: #79c0ff; }
        .chat-container { flex: 1; overflow-y: auto; padding: 20px; display: flex; flex-direction: column; gap: 16px; }
        .message { max-width: 85%; padding: 12px 16px; border-radius: 12px; line-height: 1.5; }
        .message.user { background: #1f6feb; color: white; align-self: flex-end; border-bottom-right-radius: 4px; }
        .message.assistant { background: #21262d; align-self: flex-start; border-bottom-left-radius: 4px; }
        .message .role { font-size: 11px; color: #8b949e; margin-bottom: 4px; }
        .message pre { background: #161b22; padding: 8px; border-radius: 6px; overflow-x: auto; margin: 8px 0; }
        .message code { background: #161b22; padding: 2px 4px; border-radius: 3px; font-family: monospace; }
        .input-area { background: #161b22; padding: 16px; border-top: 1px solid #30363d; display: flex; gap: 12px; }
        .input-area textarea { flex: 1; background: #0d1117; border: 1px solid #30363d; border-radius: 8px; padding: 12px; color: #c9d1d9; font-size: 14px; resize: none; min-height: 44px; max-height: 200px; font-family: inherit; }
        .input-area textarea:focus { outline: none; border-color: #58a6ff; }
        .input-area button { background: #238636; color: white; border: none; border-radius: 8px; padding: 12px 24px; cursor: pointer; font-size: 14px; font-weight: 500; }
        .input-area button:hover { background: #2ea043; }
        .input-area button:disabled { background: #21262d; color: #484f58; cursor: not-allowed; }
        .typing { display: inline-block; padding: 8px 12px; }
        .typing span { animation: blink 1.4s infinite both; }
        .typing span:nth-child(2) { animation-delay: 0.2s; }
        .typing span:nth-child(3) { animation-delay: 0.4s; }
        @keyframes blink { 0%, 80%, 100% { opacity: 0; } 40% { opacity: 1; } }
    </style>
</head>
<body>
    <div class="header">
        <h1>AnyClaw</h1>
        <span class="status" id="status">Connecting...</span>
        <span class="model" id="model"></span>
    </div>
    <div class="chat-container" id="chat"></div>
    <div class="input-area">
        <textarea id="input" placeholder="Type a message..." rows="1"></textarea>
        <button id="send" onclick="sendMessage()">Send</button>
    </div>
    <script>
        var chat = document.getElementById('chat');
        var input = document.getElementById('input');
        var sendBtn = document.getElementById('send');
        var statusEl = document.getElementById('status');
        var ws = null;
        var sessionId = null;
        var connected = false;

        function connect() {
            var protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
            ws = new WebSocket(protocol + '//' + location.host + '/ws');
            ws.onopen = function() { statusEl.textContent = 'Connected'; connected = true; };
            ws.onmessage = function(event) { handleMessage(JSON.parse(event.data)); };
            ws.onclose = function() { statusEl.textContent = 'Disconnected'; connected = false; setTimeout(connect, 3000); };
            ws.onerror = function() { statusEl.textContent = 'Error'; };
        }

        function handleMessage(data) {
            if (data.type === 'event') {
                if (data.event === 'connect.challenge') {
                    ws.send(JSON.stringify({ type: 'req', id: 'c1', method: 'connect', params: { nonce: data.data.nonce } }));
                } else if (data.event === 'chat.started') {
                    showTyping();
                } else if (data.event === 'chat.completed') {
                    hideTyping();
                }
            } else if (data.type === 'res') {
                if (data.ok && data.data) {
                    if (data.data.response) addMessage('assistant', data.data.response);
                    if (data.data.session) sessionId = data.data.session.id;
                }
            }
        }

        function sendMessage() {
            var text = input.value.trim();
            if (!text || !connected) return;
            addMessage('user', text);
            input.value = '';
            sendBtn.disabled = true;
            ws.send(JSON.stringify({ type: 'req', id: 'chat-' + Date.now(), method: 'chat.send', params: { message: text, session_id: sessionId || '' } }));
            setTimeout(function() { sendBtn.disabled = false; }, 1000);
        }

        function addMessage(role, content) {
            var div = document.createElement('div');
            div.className = 'message ' + role;
            div.innerHTML = '<div class="role">' + role + '</div>' + content.replace(/\n/g, '<br>');
            chat.appendChild(div);
            chat.scrollTop = chat.scrollHeight;
        }

        function showTyping() {
            var div = document.createElement('div');
            div.className = 'message assistant';
            div.id = 'typing';
            div.innerHTML = '<div class="typing"><span>.</span><span>.</span><span>.</span></div>';
            chat.appendChild(div);
            chat.scrollTop = chat.scrollHeight;
        }

        function hideTyping() {
            var typing = document.getElementById('typing');
            if (typing) typing.remove();
        }

        input.addEventListener('keydown', function(e) { if (e.key === 'Enter' && !e.shift) { e.preventDefault(); sendMessage(); } });
        connect();
    </script>
</body>
</html>`

// handleWebChat serves the embedded WebChat UI
func (s *Server) handleWebChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(webChatHTML))
}

// handleWebChatStatus returns the current status for WebChat
func (s *Server) handleWebChatStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := s.status()
	writeJSON(w, http.StatusOK, map[string]any{
		"model":    status.Model,
		"provider": status.Provider,
		"version":  status.Version,
		"skills":   status.Skills,
		"tools":    status.Tools,
	})
}
