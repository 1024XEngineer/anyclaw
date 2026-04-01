package web

const A2UIHTML = `<!DOCTYPE html>
<html>
<head>
    <title>AnyClaw Canvas (A2UI)</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        html, body { height: 100%; overflow: hidden; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0d1117; color: #c9d1d9; }
        #app { height: 100%; display: flex; flex-direction: column; }
        header { background: #161b22; padding: 12px 20px; border-bottom: 1px solid #30363d; display: flex; justify-content: space-between; align-items: center; }
        h1 { color: #58a6ff; font-size: 18px; }
        .tabs { display: flex; gap: 4px; }
        .tab { background: transparent; color: #8b949e; border: none; padding: 8px 16px; border-radius: 6px; cursor: pointer; font-size: 13px; }
        .tab:hover { background: #21262d; color: #c9d1d9; }
        .tab.active { background: #238636; color: white; }
        .toolbar { display: flex; gap: 8px; }
        .btn { background: #238636; color: white; border: none; padding: 6px 12px; border-radius: 6px; cursor: pointer; font-size: 12px; }
        .btn:hover { background: #2ea043; }
        .btn.danger { background: #da3633; }
        .btn.danger:hover { background: #f85149; }
        .btn.secondary { background: #21262d; border: 1px solid #30363d; }
        .btn.secondary:hover { background: #30363d; }
        .split-view { flex: 1; display: flex; overflow: hidden; }
        .panel { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
        .panel-header { background: #161b22; padding: 8px 16px; border-bottom: 1px solid #30363d; font-size: 12px; color: #8b949e; text-transform: uppercase; }
        #canvas-container { flex: 1; overflow: auto; padding: 20px; background: #0d1117; }
        #canvas { min-height: 100%; background: #161b22; border-radius: 8px; padding: 20px; white-space: pre-wrap; font-family: 'SF Mono', Monaco, monospace; font-size: 14px; line-height: 1.6; color: #e6edf3; }
        #terminal { height: 200px; background: #0d1117; border-top: 1px solid #30363d; padding: 10px; font-family: 'SF Mono', Monaco, monospace; font-size: 12px; overflow-y: auto; }
        .log-entry { margin: 2px 0; }
        .log-entry.info { color: #58a6ff; }
        .log-entry.success { color: #3fb950; }
        .log-entry.error { color: #f85149; }
        #input-area { background: #161b22; border-top: 1px solid #30363d; padding: 10px; display: flex; gap: 8px; }
        #code-input, #message-input { flex: 1; background: #0d1117; border: 1px solid #30363d; border-radius: 6px; color: #c9d1d9; padding: 8px; font-size: 12px; }
        #code-input:focus, #message-input:focus { outline: none; border-color: #58a6ff; }
        .tool-panel { width: 300px; border-left: 1px solid #30363d; background: #161b22; overflow-y: auto; }
        .tool-section { padding: 12px; border-bottom: 1px solid #30363d; }
        .tool-section h3 { color: #8b949e; font-size: 11px; text-transform: uppercase; margin-bottom: 8px; }
        .tool-item { display: block; padding: 8px; margin: 4px 0; background: #21262d; border-radius: 6px; color: #c9d1d9; text-decoration: none; font-size: 12px; cursor: pointer; }
        .tool-item:hover { background: #30363d; }
        .tool-item .name { font-weight: 500; }
        .tool-item .desc { color: #8b949e; font-size: 11px; }
        .a2-badge { display: inline-block; background: #a371f7; color: white; padding: 2px 6px; border-radius: 4px; font-size: 10px; margin-left: 8px; }
        .thinking { color: #8b949e; font-style: italic; }
        .message { margin: 8px 0; padding: 8px; border-radius: 6px; }
        .message.user { background: #1f6feb; margin-left: 20%; }
        .message.assistant { background: #21262d; margin-right: 20%; }
        .tool-call { background: #2d333b; border-left: 3px solid #a371f7; padding: 8px; margin: 4px 0; font-size: 12px; }
        .tool-call .name { color: #a371f7; font-weight: 500; }
    </style>
</head>
<body>
<div id="app">
<header>
<div style="display:flex;align-items:center;gap:12px;">
<h1>Canvas</h1>
<span class="a2-badge">A2UI</span>
</div>
<div class="tabs">
<button class="tab active" onclick="switchTab('chat')">Chat</button>
<button class="tab" onclick="switchTab('canvas')">Canvas</button>
<button class="tab" onclick="switchTab('tools')">Tools</button>
</div>
<div class="toolbar">
<button class="btn secondary" onclick="clearCanvas()">Clear</button>
<button class="btn secondary" onclick="takeSnapshot()">Snapshot</button>
<button class="btn" onclick="pushContent()">Push</button>
</div>
</header>

<div id="chat-view" class="split-view">
<div class="panel">
<div class="panel-header">Conversation</div>
<div id="messages" style="flex:1;overflow-y:auto;padding:12px;">
<div class="message assistant"><div class="thinking">Thinking...</div></div>
</div>
<div id="input-area">
<input type="text" id="message-input" placeholder="Send a message..." onkeypress="handleMessage(event)">
<button class="btn" onclick="sendMessage()">Send</button>
</div>
</div>
</div>

<div id="canvas-view" class="split-view" style="display:none;">
<div class="panel">
<div class="panel-header">Canvas</div>
<div id="canvas-container"><div id="canvas"></div></div>
<div id="terminal"></div>
<div id="input-area">
<input type="text" id="code-input" placeholder="JavaScript eval..." onkeypress="handleInput(event)">
<button class="btn" onclick="evalCode()">Eval</button>
</div>
</div>
<div class="tool-panel">
<div class="tool-section"><h3>Tools</h3>
<div class="tool-item" onclick="callTool('read_file')"><div class="name">read_file</div><div class="desc">Read file contents</div></div>
<div class="tool-item" onclick="callTool('write_file')"><div class="name">write_file</div><div class="desc">Write to file</div></div>
<div class="tool-item" onclick="callTool('run_command')"><div class="name">run_command</div><div class="desc">Execute shell command</div></div>
<div class="tool-item" onclick="callTool('browser_navigate')"><div class="name">browser_navigate</div><div class="desc">Navigate browser</div></div>
</div>
<div class="tool-section"><h3>Quick Push</h3>
<button class="btn secondary" style="width:100%;margin:4px 0;" onclick="pushMarkdown()">Markdown</button>
<button class="btn secondary" style="width:100%;margin:4px 0;" onclick="pushJSON()">JSON</button>
<button class="btn secondary" style="width:100%;margin:4px 0;" onclick="pushCode()">Code</button>
</div>
</div>
</div>

<div id="tools-view" class="split-view" style="display:none;">
<div class="panel" style="flex:2;">
<div class="panel-header">Tool Registry</div>
<div style="padding:12px;">
<div class="tool-item"><div class="name">read_file</div><div class="desc">Read file contents from filesystem</div></div>
<div class="tool-item"><div class="name">write_file</div><div class="desc">Write content to file</div></div>
<div class="tool-item"><div class="name">run_command</div><div class="desc">Execute shell commands</div></div>
<div class="tool-item"><div class="name">browser_navigate</div><div class="desc">Navigate browser to URL</div></div>
<div class="tool-item"><div class="name">canvas_push</div><div class="desc">Push content to canvas</div></div>
<div class="tool-item"><div class="name">canvas_eval</div><div class="desc">Evaluate JavaScript</div></div>
</div>
</div>
<div class="tool-panel">
<div class="tool-section"><h3>Actions</h3>
<button class="btn" style="width:100%;margin:4px 0;" onclick="resetCanvas()">Reset Canvas</button>
<button class="btn secondary" style="width:100%;margin:4px 0;" onclick="getCanvasState()">Get State</button>
</div>
</div>
</div>
</div>

<script>
var ws = null;
var content = '';

function connect() {
    ws = new WebSocket('ws://' + location.host + '/canvas/ws');
    ws.onopen = function() { log('Connected', 'success'); };
    ws.onmessage = function(e) { handleWSMessage(JSON.parse(e.data)); };
    ws.onclose = function() { log('Disconnected', 'error'); setTimeout(connect, 3000); };
}

function handleWSMessage(data) {
    if (data.type === 'canvas_state' || data.type === 'canvas_push') {
        content = data.content || '';
        document.getElementById('canvas').textContent = content;
    } else if (data.type === 'canvas_reset') {
        content = ''; document.getElementById('canvas').textContent = '';
    } else if (data.type === 'canvas_eval') {
        log('eval: ' + data.code, 'info');
        log('=> ' + data.result, 'success');
    }
}

function switchTab(tab) {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    event.target.classList.add('active');
    document.getElementById('chat-view').style.display = tab === 'chat' ? 'flex' : 'none';
    document.getElementById('canvas-view').style.display = tab === 'canvas' ? 'flex' : 'none';
    document.getElementById('tools-view').style.display = tab === 'tools' ? 'flex' : 'none';
}

function pushContent() {
    var c = prompt('Content:');
    if (!c) return;
    fetch('/canvas/api/push', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({content: c, reset: false})})
    .then(r => r.json()).then(d => log('Pushed ' + d.length + ' chars', 'success'));
}

function pushMarkdown() {
    fetch('/canvas/api/push', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({content: '# Heading\n\nContent', reset: false})});
    log('Pushed markdown', 'success');
}

function pushJSON() {
    fetch('/canvas/api/push', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({content: '{"key":"value"}', reset: false})});
    log('Pushed JSON', 'success');
}

function pushCode() {
    fetch('/canvas/api/push', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({content: 'console.log("Hello");', reset: false})});
    log('Pushed code', 'success');
}

function resetCanvas() { fetch('/canvas/api/reset', {method: 'POST'}).then(r => r.json()).then(d => log('Reset', 'success')); }
function clearCanvas() { if (confirm('Clear?')) resetCanvas(); }
function takeSnapshot() { fetch('/canvas/api/snapshot').then(r => r.json()).then(d => log('Snapshot: ' + d.snapshot.length, 'success')); }
function getCanvasState() { fetch('/canvas/api/state').then(r => r.json()).then(d => log('State: ' + d.content.length, 'info')); }

function evalCode() {
    var code = document.getElementById('code-input').value;
    if (!code) return;
    fetch('/canvas/api/eval', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({code: code})});
    document.getElementById('code-input').value = '';
}
function handleInput(e) { if (e.key === 'Enter') evalCode(); }

function sendMessage() {
    var input = document.getElementById('message-input');
    var c = input.value;
    if (!c) return;
    addMessage('user', c);
    input.value = '';
}
function handleMessage(e) { if (e.key === 'Enter') sendMessage(); }

function addMessage(role, text) {
    var div = document.createElement('div');
    div.className = 'message ' + role;
    div.textContent = text;
    document.getElementById('messages').appendChild(div);
    document.getElementById('messages').scrollTop = document.getElementById('messages').scrollHeight;
}

function callTool(name) {
    var inp = prompt('Input for ' + name + ' (JSON):');
    if (!inp) return;
    try { JSON.parse(inp); } catch(e) { alert('Invalid JSON'); return; }
    fetch('/api/tools/invoke', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({tool: name, input: JSON.parse(inp)})});
}

function log(msg, type) {
    var div = document.createElement('div');
    div.className = 'log-entry ' + type;
    div.textContent = '[' + new Date().toLocaleTimeString() + '] ' + msg;
    document.getElementById('terminal').appendChild(div);
    document.getElementById('terminal').scrollTop = document.getElementById('terminal').scrollHeight;
}

connect();
</script>
</body>
</html>`
