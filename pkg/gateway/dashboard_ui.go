package gateway

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
)

var dashboardHTML = dashboardHeadHTML + dashboardBodyHTML + dashboardScriptAHTML + dashboardScriptBHTML + dashboardScriptCHTML

const dashboardHeadHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AnyClaw Console</title>
    <style>
        :root {
            --bg: #f4ede2;
            --panel: rgba(255, 251, 245, 0.92);
            --panel-strong: rgba(255, 255, 255, 0.72);
            --line: rgba(80, 63, 42, 0.14);
            --text: #1c1a18;
            --muted: #6e655a;
            --accent: #0f766e;
            --accent-2: #115e59;
            --accent-soft: rgba(15, 118, 110, 0.12);
            --warn: #b45309;
            --warn-soft: rgba(180, 83, 9, 0.12);
            --danger: #b42318;
            --danger-soft: rgba(180, 35, 24, 0.12);
            --shadow: 0 18px 42px rgba(47, 35, 21, 0.12);
            --radius: 18px;
            --mono: "Cascadia Code", Consolas, monospace;
            --sans: "Aptos", "Segoe UI Variable", "Segoe UI", sans-serif;
        }

        * { box-sizing: border-box; }

        body {
            margin: 0;
            min-height: 100vh;
            font-family: var(--sans);
            color: var(--text);
            background:
                radial-gradient(circle at top left, rgba(15, 118, 110, 0.18), transparent 30%),
                radial-gradient(circle at bottom right, rgba(180, 83, 9, 0.12), transparent 30%),
                linear-gradient(180deg, #f7f1e6 0%, #f2e8d9 100%);
        }

        body::before {
            content: "";
            position: fixed;
            inset: 0;
            pointer-events: none;
            opacity: 0.32;
            background-image:
                linear-gradient(rgba(85, 67, 44, 0.03) 1px, transparent 1px),
                linear-gradient(90deg, rgba(85, 67, 44, 0.03) 1px, transparent 1px);
            background-size: 28px 28px;
        }

        .shell {
            position: relative;
            z-index: 1;
            display: grid;
            grid-template-columns: 280px minmax(0, 1fr);
            min-height: 100vh;
        }

        .sidebar {
            padding: 24px 20px;
            border-right: 1px solid var(--line);
            background: rgba(255, 248, 239, 0.72);
            backdrop-filter: blur(14px);
            display: flex;
            flex-direction: column;
            gap: 16px;
        }

        .brand {
            padding: 16px;
            border-radius: var(--radius);
            color: #f7fffd;
            background: linear-gradient(135deg, rgba(17, 94, 89, 0.95), rgba(15, 118, 110, 0.82));
            box-shadow: var(--shadow);
        }

        .brand h1 {
            margin: 0 0 6px;
            font-size: 22px;
        }

        .brand p {
            margin: 0;
            font-size: 13px;
            line-height: 1.5;
            color: rgba(247, 255, 253, 0.82);
        }

        .nav {
            display: grid;
            gap: 10px;
        }

        .nav button {
            width: 100%;
            text-align: left;
            padding: 14px 15px;
            border-radius: 16px;
            border: 1px solid transparent;
            background: transparent;
            color: var(--text);
            font: inherit;
            cursor: pointer;
            transition: 180ms ease;
        }

        .nav button:hover {
            transform: translateX(2px);
            background: rgba(255, 255, 255, 0.5);
            border-color: rgba(15, 118, 110, 0.16);
        }

        .nav button.active {
            background: var(--panel);
            border-color: rgba(15, 118, 110, 0.18);
            box-shadow: 0 10px 24px rgba(31, 21, 12, 0.08);
        }

        .nav-title {
            display: block;
            font-size: 15px;
            font-weight: 700;
            margin-bottom: 4px;
        }

        .nav-copy {
            display: block;
            color: var(--muted);
            font-size: 12px;
            line-height: 1.45;
        }

        .side-tools {
            margin-top: auto;
            display: grid;
            gap: 12px;
            padding: 16px;
            border-radius: 16px;
            background: var(--panel);
            border: 1px solid rgba(15, 118, 110, 0.1);
        }

        .btn {
            border: none;
            border-radius: 14px;
            padding: 11px 14px;
            color: white;
            font: inherit;
            font-weight: 700;
            cursor: pointer;
            background: linear-gradient(135deg, var(--accent), var(--accent-2));
            box-shadow: 0 10px 24px rgba(15, 118, 110, 0.2);
        }

        .btn.secondary {
            color: var(--accent-2);
            background: rgba(17, 94, 89, 0.08);
            border: 1px solid rgba(15, 118, 110, 0.14);
            box-shadow: none;
        }

        .toggle {
            display: flex;
            align-items: center;
            gap: 10px;
            color: var(--muted);
            font-size: 13px;
        }

        .main {
            padding: 26px;
            display: grid;
            gap: 18px;
            min-width: 0;
        }

        .topbar {
            display: flex;
            justify-content: space-between;
            gap: 16px;
            align-items: flex-start;
        }

        .topbar h2 {
            margin: 0 0 6px;
            font-size: 28px;
        }

        .topbar p {
            margin: 0;
            color: var(--muted);
            line-height: 1.5;
        }

        .topbar-meta {
            display: flex;
            flex-wrap: wrap;
            gap: 10px;
        }

        .pill,
        .tag {
            display: inline-flex;
            align-items: center;
            padding: 7px 11px;
            border-radius: 999px;
            font-size: 12px;
            font-weight: 700;
            border: 1px solid transparent;
        }

        .pill {
            background: var(--panel);
            border-color: rgba(15, 118, 110, 0.1);
        }

        .tag.ok {
            background: var(--accent-soft);
            color: var(--accent-2);
        }

        .tag.warn {
            background: var(--warn-soft);
            color: var(--warn);
        }

        .tag.error {
            background: var(--danger-soft);
            color: var(--danger);
        }

        .tag.neutral {
            background: rgba(76, 60, 40, 0.08);
            color: var(--muted);
        }

        .notice {
            display: none;
            padding: 14px 16px;
            border-radius: 16px;
            background: var(--panel);
            border: 1px solid rgba(15, 118, 110, 0.12);
        }

        .notice.show { display: block; }
        .notice.error {
            color: var(--danger);
            background: rgba(255, 246, 245, 0.96);
            border-color: rgba(180, 35, 24, 0.16);
        }

        .notice.success {
            color: var(--text);
            background: rgba(255, 255, 255, 0.76);
        }

        .panel { display: none; }
        .panel.active { display: block; }

        .grid {
            display: grid;
            gap: 18px;
        }

        .grid.two {
            grid-template-columns: repeat(2, minmax(0, 1fr));
        }

        .split {
            display: grid;
            grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
            gap: 18px;
        }

        .card {
            min-width: 0;
            padding: 18px;
            border-radius: var(--radius);
            background: var(--panel);
            border: 1px solid rgba(78, 61, 41, 0.11);
            box-shadow: var(--shadow);
        }

        .card-head {
            display: flex;
            justify-content: space-between;
            align-items: center;
            gap: 10px;
            margin-bottom: 14px;
        }

        .card h3 {
            margin: 0;
            font-size: 17px;
        }

        .card-copy,
        .meta,
        .subtle {
            color: var(--muted);
        }

        .card-copy {
            font-size: 13px;
            line-height: 1.5;
            margin-bottom: 12px;
        }

        .metrics {
            display: grid;
            grid-template-columns: repeat(4, minmax(0, 1fr));
            gap: 14px;
        }

        .metric {
            padding: 15px;
            border-radius: 16px;
            background: var(--panel-strong);
            border: 1px solid rgba(15, 118, 110, 0.08);
        }

        .metric span {
            display: block;
            margin-bottom: 8px;
            font-size: 12px;
            text-transform: uppercase;
            letter-spacing: 0.08em;
            color: var(--muted);
        }

        .metric strong {
            display: block;
            font-size: 19px;
            line-height: 1.25;
            word-break: break-word;
        }

        .metric small {
            display: block;
            margin-top: 8px;
            color: var(--muted);
            line-height: 1.45;
        }

        .list {
            display: grid;
            gap: 12px;
        }

        .item {
            padding: 14px 15px;
            border-radius: 16px;
            background: rgba(255, 255, 255, 0.58);
            border: 1px solid rgba(78, 61, 41, 0.08);
        }

        .item-head {
            display: flex;
            justify-content: space-between;
            gap: 12px;
            align-items: flex-start;
            margin-bottom: 10px;
        }

        .item-title {
            margin: 0;
            font-size: 15px;
            line-height: 1.35;
        }

        .item-subtitle {
            margin-top: 4px;
            font-size: 12px;
            color: var(--muted);
        }

        .meta {
            display: flex;
            flex-wrap: wrap;
            gap: 8px 10px;
            font-size: 12px;
        }

        .stack {
            display: grid;
            gap: 8px;
        }

        details {
            margin-top: 12px;
            padding-top: 12px;
            border-top: 1px solid rgba(78, 61, 41, 0.08);
        }

        details summary {
            cursor: pointer;
            color: var(--accent-2);
            font-weight: 700;
        }

        .log-json,
        pre {
            margin: 0;
            white-space: pre-wrap;
            word-break: break-word;
            font-family: var(--mono);
            font-size: 12px;
            line-height: 1.55;
        }

        .json-box {
            max-height: 460px;
            overflow: auto;
            padding: 16px;
            border-radius: 16px;
            background: #1e1d22;
            color: #f7f5f0;
            box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.06);
        }

        .error-box {
            padding: 14px 16px;
            border-radius: 16px;
            background: rgba(180, 35, 24, 0.08);
            border: 1px solid rgba(180, 35, 24, 0.12);
            color: var(--danger);
            line-height: 1.5;
        }

        .empty {
            padding: 18px;
            text-align: center;
            color: var(--muted);
            background: rgba(255, 255, 255, 0.48);
            border: 1px dashed rgba(78, 61, 41, 0.18);
            border-radius: 16px;
        }

        .toolbar {
            display: flex;
            flex-wrap: wrap;
            gap: 10px;
            margin-bottom: 14px;
        }

        input[type="text"],
        select {
            width: 100%;
            padding: 12px 13px;
            border-radius: 14px;
            border: 1px solid rgba(78, 61, 41, 0.14);
            background: rgba(255, 255, 255, 0.82);
            color: var(--text);
            font: inherit;
        }

        label {
            display: grid;
            gap: 8px;
            font-size: 13px;
            color: var(--muted);
        }

        .actions {
            display: flex;
            flex-wrap: wrap;
            gap: 10px;
            margin-top: 14px;
        }

        @media (max-width: 1180px) {
            .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }
            .grid.two,
            .split { grid-template-columns: 1fr; }
        }

        @media (max-width: 920px) {
            .shell { grid-template-columns: 1fr; }
            .sidebar {
                border-right: none;
                border-bottom: 1px solid var(--line);
            }
            .nav { grid-template-columns: repeat(2, minmax(0, 1fr)); }
        }

        @media (max-width: 640px) {
            .main { padding: 18px; }
            .sidebar { padding: 18px; }
            .nav { grid-template-columns: 1fr; }
            .metrics { grid-template-columns: 1fr; }
            .topbar { flex-direction: column; }
        }
    </style>
</head>
`

const dashboardBodyHTML = `<body>
    <div class="shell">
        <aside class="sidebar">
            <div class="brand">
                <h1>AnyClaw Console</h1>
                <p>Status, sessions, logs, skills, and config in one place.</p>
            </div>

            <nav class="nav">
                <button data-tab="status" class="active">
                    <span class="nav-title">Status</span>
                    <span class="nav-copy">Gateway health, providers, runtimes, channels</span>
                </button>
                <button data-tab="sessions">
                    <span class="nav-title">Sessions</span>
                    <span class="nav-copy">Recent sessions, message preview, workspace spread</span>
                </button>
                <button data-tab="logs">
                    <span class="nav-title">Logs</span>
                    <span class="nav-copy">Audit, events, tool activity, background jobs</span>
                </button>
                <button data-tab="skills">
                    <span class="nav-title">Skills</span>
                    <span class="nav-copy">Loaded skills, plugins, and agent bindings</span>
                </button>
                <button data-tab="config">
                    <span class="nav-title">Config</span>
                    <span class="nav-copy">Current config snapshot and provider actions</span>
                </button>
                <button data-tab="nodes">
                    <span class="nav-title">Nodes</span>
                    <span class="nav-copy">Connected nodes, pairing, capabilities, devices</span>
                </button>
                <button data-tab="channels">
                    <span class="nav-title">Channels</span>
                    <span class="nav-copy">Telegram, Discord, Slack, WhatsApp, Signal</span>
                </button>
                <button data-tab="remote">
                    <span class="nav-title">Remote</span>
                    <span class="nav-copy">Tailscale, SSH tunnels, remote gateway</span>
                </button>
            </nav>

            <div class="side-tools">
                <button class="btn" id="refreshCurrent">Refresh current page</button>
                <label class="toggle">
                    <input type="checkbox" id="autoRefresh" checked>
                    <span>Auto refresh every 15s</span>
                </label>
                <div class="subtle">
                    This dashboard reads the existing JSON APIs. If one panel shows a permission error,
                    the current user token does not have access to that endpoint.
                </div>
            </div>
        </aside>

        <main class="main">
            <header class="topbar">
                <div>
                    <h2 id="pageTitle">Status</h2>
                    <p id="pageSubtitle">Gateway health, provider routing, runtime state, and channels.</p>
                </div>
                <div class="topbar-meta">
                    <span class="pill" id="versionPill">Version --</span>
                    <span class="pill" id="updatedPill">Not refreshed yet</span>
                </div>
            </header>

            <div class="notice" id="notice"></div>

            <section class="panel active" data-panel="status">
                <div class="card">
                    <div class="card-head">
                        <h3>Overview</h3>
                        <span class="tag neutral" id="statusHealthTag">Waiting</span>
                    </div>
                    <div class="metrics" id="statusMetrics"></div>
                </div>
                <div class="grid two">
                    <div class="card">
                        <div class="card-head">
                            <h3>Providers</h3>
                            <span class="tag neutral" id="providerCountTag">0</span>
                        </div>
                        <div class="list" id="providerList"></div>
                    </div>
                    <div class="card">
                        <div class="card-head">
                            <h3>Channels</h3>
                            <span class="tag neutral" id="channelCountTag">0</span>
                        </div>
                        <div class="list" id="channelList"></div>
                    </div>
                </div>
                <div class="grid two">
                    <div class="card">
                        <div class="card-head">
                            <h3>Runtimes</h3>
                            <span class="tag neutral" id="runtimeCountTag">0</span>
                        </div>
                        <div class="list" id="runtimeList"></div>
                    </div>
                    <div class="card">
                        <div class="card-head">
                            <h3>Recent events</h3>
                            <span class="tag neutral">Control plane</span>
                        </div>
                        <div class="list" id="statusEventList"></div>
                    </div>
                </div>
            </section>

            <section class="panel" data-panel="sessions">
                <div class="card">
                    <div class="card-head">
                        <h3>Sessions</h3>
                        <span class="tag neutral" id="sessionCountTag">0</span>
                    </div>
                    <div class="toolbar">
                        <input type="text" id="sessionSearch" placeholder="Search by title, agent, workspace, or preview text">
                    </div>
                    <div class="list" id="sessionList"></div>
                </div>
            </section>

            <section class="panel" data-panel="logs">
                <div class="grid two">
                    <div class="card">
                        <div class="card-head">
                            <h3>Audit</h3>
                            <span class="tag neutral" id="auditCountTag">0</span>
                        </div>
                        <div class="list" id="auditList"></div>
                    </div>
                    <div class="card">
                        <div class="card-head">
                            <h3>Events</h3>
                            <span class="tag neutral" id="eventCountTag">0</span>
                        </div>
                        <div class="list" id="eventList"></div>
                    </div>
                </div>
                <div class="grid two">
                    <div class="card">
                        <div class="card-head">
                            <h3>Tool activity</h3>
                            <span class="tag neutral" id="toolCountTag">0</span>
                        </div>
                        <div class="list" id="toolList"></div>
                    </div>
                    <div class="card">
                        <div class="card-head">
                            <h3>Jobs</h3>
                            <span class="tag neutral" id="jobCountTag">0</span>
                        </div>
                        <div class="list" id="jobList"></div>
                    </div>
                </div>
            </section>

            <section class="panel" data-panel="skills">
                <div class="grid two">
                    <div class="card">
                        <div class="card-head">
                            <h3>Skills</h3>
                            <span class="tag neutral" id="skillCountTag">0</span>
                        </div>
                        <div class="list" id="skillList"></div>
                    </div>
                    <div class="card">
                        <div class="card-head">
                            <h3>Plugins</h3>
                            <span class="tag neutral" id="pluginCountTag">0</span>
                        </div>
                        <div class="list" id="pluginList"></div>
                    </div>
                </div>
                <div class="card">
                    <div class="card-head">
                        <h3>Agent bindings</h3>
                        <span class="tag neutral" id="bindingCountTag">0</span>
                    </div>
                    <div class="list" id="bindingList"></div>
                </div>
            </section>

            <section class="panel" data-panel="config">
                <div class="split">
                    <div class="card">
                        <div class="card-head">
                            <h3>Default provider</h3>
                            <span class="tag neutral" id="configProviderStatus">Waiting</span>
                        </div>
                        <p class="card-copy">
                            Switch the default provider profile or run a quick connectivity test.
                        </p>
                        <label>
                            <span>Provider</span>
                            <select id="defaultProviderSelect"></select>
                        </label>
                        <div class="actions">
                            <button class="btn" id="saveDefaultProvider">Set default</button>
                            <button class="btn secondary" id="testDefaultProvider">Test provider</button>
                        </div>
                        <div class="subtle" id="providerActionNote">No provider action yet.</div>
                    </div>

                    <div class="card">
                        <div class="card-head">
                            <h3>Global fallback model</h3>
                            <span class="tag neutral" id="configModelStatus">Waiting</span>
                        </div>
                        <p class="card-copy">
                            This only updates the global llm.model fallback value. It does not override a provider profile default model.
                        </p>
                        <label>
                            <span>Model name</span>
                            <input type="text" id="globalModelInput" placeholder="Example: gpt-5.4-mini">
                        </label>
                        <div class="actions">
                            <button class="btn" id="saveGlobalModel">Save model</button>
                        </div>
                        <div class="subtle" id="modelActionNote">No model action yet.</div>
                    </div>
                </div>

                <div class="card">
                    <div class="card-head">
                        <h3>Config snapshot</h3>
                        <span class="tag neutral" id="configSizeTag">0</span>
                    </div>
                    <div class="json-box">
                        <pre id="configJSON">{}</pre>
                    </div>
                </div>
            </section>

            <section class="panel" data-panel="nodes">
                <div class="split">
                    <div class="card">
                        <div class="card-head">
                            <h3>Node Pairing</h3>
                            <span class="tag neutral" id="nodePairingStatus">Idle</span>
                        </div>
                        <p class="card-copy">Generate a pairing code for new nodes to connect.</p>
                        <div class="actions">
                            <button class="btn" id="generatePairingCode">Generate Code</button>
                            <button class="btn secondary" id="revokePairing">Revoke Selected</button>
                        </div>
                        <div class="subtle" id="pairingCodeDisplay">No active pairing code.</div>
                    </div>
                    <div class="card">
                        <div class="card-head">
                            <h3>Connected Nodes</h3>
                            <span class="tag neutral" id="nodeCount">0</span>
                        </div>
                        <div id="nodesList">
                            <p class="muted">No nodes connected.</p>
                        </div>
                    </div>
                </div>
            </section>

            <section class="panel" data-panel="channels">
                <div class="split">
                    <div class="card">
                        <div class="card-head">
                            <h3>Channel Status</h3>
                            <span class="tag neutral" id="channelCount">0</span>
                        </div>
                        <div id="channelsList">
                            <p class="muted">No channels configured.</p>
                        </div>
                    </div>
                    <div class="card">
                        <div class="card-head">
                            <h3>Security Policy</h3>
                            <span class="tag neutral" id="securityStatus">Check</span>
                        </div>
                        <div id="securityInfo">
                            <p class="muted">Run doctor to check security.</p>
                        </div>
                    </div>
                </div>
            </section>

            <section class="panel" data-panel="remote">
                <div class="split">
                    <div class="card">
                        <div class="card-head">
                            <h3>Remote Access</h3>
                            <span class="tag neutral" id="remoteStatus">Idle</span>
                        </div>
                        <p class="card-copy">Configure remote access via Tailscale, SSH tunnel, or gateway.</p>
                        <label>
                            <span>Mode</span>
                            <select id="remoteMode">
                                <option value="gateway">Remote Gateway</option>
                                <option value="tailscale">Tailscale</option>
                                <option value="ssh">SSH Tunnel</option>
                            </select>
                        </label>
                        <label>
                            <span>Endpoint URL</span>
                            <input type="text" id="remoteEndpoint" placeholder="https://your-gateway.example.com">
                        </label>
                        <div class="actions">
                            <button class="btn" id="connectRemote">Connect</button>
                            <button class="btn secondary" id="disconnectRemote">Disconnect</button>
                        </div>
                    </div>
                    <div class="card">
                        <div class="card-head">
                            <h3>Remote Nodes</h3>
                            <span class="tag neutral" id="remoteNodeCount">0</span>
                        </div>
                        <div id="remoteNodesList">
                            <p class="muted">No remote nodes connected.</p>
                        </div>
                    </div>
                </div>
            </section>
        </main>
    </div>
`

const dashboardScriptAHTML = `<script>
        var tabMeta = {
            status: { title: 'Status', subtitle: 'Gateway health, provider routing, runtime state, and channels.' },
            sessions: { title: 'Sessions', subtitle: 'Recent sessions, message previews, and workspace placement.' },
            logs: { title: 'Logs', subtitle: 'Audit entries, events, tool activity, and background jobs.' },
            skills: { title: 'Skills', subtitle: 'Loaded skills, plugins, and agent-to-provider bindings.' },
            config: { title: 'Config', subtitle: 'Current config snapshot plus a few high-frequency actions.' },
            nodes: { title: 'Nodes', subtitle: 'Connected nodes, pairing status, capabilities, and device state.' },
            channels: { title: 'Channels', subtitle: 'Telegram, Discord, Slack, WhatsApp, Signal connection status.' },
            remote: { title: 'Remote', subtitle: 'Tailscale, SSH tunnels, and remote gateway connections.' }
        };

        var state = {
            tab: 'status',
            sessions: [],
            providers: [],
            config: null
        };
        var refreshHandle = null;

        function pick(obj, keys, fallback) {
            if (!obj) return fallback;
            for (var i = 0; i < keys.length; i++) {
                var key = keys[i];
                if (obj[key] !== undefined && obj[key] !== null && obj[key] !== '') return obj[key];
            }
            return fallback;
        }

        function escapeHTML(value) {
            return String(value == null ? '' : value).replace(/[&<>"']/g, function(ch) {
                return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch];
            });
        }

        function prettyJSON(value) {
            return escapeHTML(JSON.stringify(value == null ? {} : value, null, 2));
        }

        function toArray(value) {
            return Array.isArray(value) ? value : [];
        }

        function formatTime(value) {
            if (!value) return '--';
            var dt = new Date(value);
            if (isNaN(dt.getTime())) return escapeHTML(String(value));
            return escapeHTML(dt.toLocaleString());
        }

        function tag(text, tone) {
            return '<span class="tag ' + tone + '">' + escapeHTML(text) + '</span>';
        }

        function setText(id, value) {
            var el = document.getElementById(id);
            if (el) el.textContent = value;
        }

        function setHTML(id, value) {
            var el = document.getElementById(id);
            if (el) el.innerHTML = value;
        }

        function showNotice(kind, message) {
            var el = document.getElementById('notice');
            if (!message) {
                el.className = 'notice';
                el.textContent = '';
                return;
            }
            el.className = 'notice show ' + kind;
            el.textContent = message;
        }

        async function requestJSON(path, options) {
            var opts = options || {};
            var headers = { 'Accept': 'application/json' };
            if (opts.body) headers['Content-Type'] = 'application/json';
            var resp = await fetch(path, {
                method: opts.method || 'GET',
                headers: headers,
                body: opts.body ? JSON.stringify(opts.body) : undefined,
                credentials: 'same-origin'
            });
            var text = await resp.text();
            var data = null;
            if (text) {
                try { data = JSON.parse(text); } catch (err) { data = text; }
            }
            if (!resp.ok) {
                var message = pick(data, ['error', 'message'], '') || text || (resp.status + ' ' + resp.statusText);
                throw new Error(path + ' -> ' + message);
            }
            return data;
        }

        async function safeRequest(path, options) {
            try {
                return { ok: true, data: await requestJSON(path, options) };
            } catch (err) {
                return { ok: false, error: err.message };
            }
        }

        function renderError(message) {
            return '<div class="error-box">' + escapeHTML(message) + '</div>';
        }

        function renderEmpty(message) {
            return '<div class="empty">' + escapeHTML(message) + '</div>';
        }

        function renderList(items, emptyText) {
            return items.length ? items.join('') : renderEmpty(emptyText);
        }

        function renderMetric(label, value, hint) {
            return '<div class="metric">' +
                '<span>' + escapeHTML(label) + '</span>' +
                '<strong>' + escapeHTML(value) + '</strong>' +
                '<small>' + escapeHTML(hint || '') + '</small>' +
                '</div>';
        }

        function updateChrome(version) {
            setText('versionPill', version ? ('Version ' + version) : 'Version --');
            setText('updatedPill', 'Refreshed at ' + new Date().toLocaleTimeString());
        }

        function activateTab(tab) {
            if (!tabMeta[tab]) tab = 'status';
            state.tab = tab;
            var buttons = document.querySelectorAll('.nav button');
            for (var i = 0; i < buttons.length; i++) {
                buttons[i].classList.toggle('active', buttons[i].getAttribute('data-tab') === tab);
            }
            var panels = document.querySelectorAll('.panel');
            for (var j = 0; j < panels.length; j++) {
                panels[j].classList.toggle('active', panels[j].getAttribute('data-panel') === tab);
            }
            setText('pageTitle', tabMeta[tab].title);
            setText('pageSubtitle', tabMeta[tab].subtitle);
            if (window.location.hash !== '#' + tab) history.replaceState(null, '', '#' + tab);
        }

        function toneForProvider(item) {
            var health = pick(item, ['health'], {});
            if (health.ok || health.OK) return 'ok';
            var status = String(health.status || health.Status || '').toLowerCase();
            if (status.indexOf('error') >= 0 || status.indexOf('invalid') >= 0 || status.indexOf('missing') >= 0) return 'error';
            return 'warn';
        }

        function toneForJob(status) {
            status = String(status || '').toLowerCase();
            if (status === 'completed') return 'ok';
            if (status === 'queued' || status === 'running') return 'warn';
            if (status === 'failed' || status === 'cancelled') return 'error';
            return 'neutral';
        }

        function renderStatus(snapshotResp, providerResp) {
            if (!snapshotResp.ok) {
                setHTML('statusMetrics', renderError(snapshotResp.error));
                setHTML('providerList', renderEmpty('Provider data unavailable.'));
                setHTML('channelList', renderEmpty('Channel data unavailable.'));
                setHTML('runtimeList', renderEmpty('Runtime data unavailable.'));
                setHTML('statusEventList', renderEmpty('Event data unavailable.'));
                setText('statusHealthTag', 'Error');
                return;
            }

            var snapshot = snapshotResp.data || {};
            var status = snapshot.status || {};
            updateChrome(status.version || '');
            setText('statusHealthTag', pick(status, ['status'], 'unknown'));

            var metrics = [
                renderMetric('Status', pick(status, ['status'], 'unknown'), 'Current gateway state'),
                renderMetric('Provider', pick(status, ['provider'], '--'), 'Global fallback provider'),
                renderMetric('Model', pick(status, ['model'], '--'), 'Global fallback model'),
                renderMetric('Address', pick(status, ['address'], '--'), 'Gateway listen address'),
                renderMetric('Sessions', String(pick(status, ['sessions'], 0)), 'Current sessions in store'),
                renderMetric('Events', String(pick(status, ['events'], 0)), 'Current stored events'),
                renderMetric('Workspace', pick(status, ['working_dir'], '--'), 'Default workspace'),
                renderMetric('Work dir', pick(status, ['work_dir'], '--'), 'AnyClaw data directory')
            ];
            setHTML('statusMetrics', metrics.join(''));

            var channels = toArray(snapshot.channels);
            setText('channelCountTag', String(channels.length));
            setHTML('channelList', renderList(channels.map(function(item) {
                var running = !!pick(item, ['running', 'Running'], false);
                var healthy = !!pick(item, ['healthy', 'Healthy'], false);
                var label = healthy && running ? tag('Running', 'ok') : tag(running ? 'Degraded' : 'Stopped', running ? 'warn' : 'error');
                return '<article class="item">' +
                    '<div class="item-head">' +
                        '<div><h4 class="item-title">' + escapeHTML(pick(item, ['name', 'Name'], 'channel')) + '</h4>' +
                        '<div class="item-subtitle">Enabled: ' + escapeHTML(String(!!pick(item, ['enabled', 'Enabled'], false))) + '</div></div>' +
                        label +
                    '</div>' +
                    '<div class="meta"><span>Last activity: ' + formatTime(pick(item, ['last_activity', 'LastActivity'], '')) + '</span></div>' +
                    (pick(item, ['last_error', 'LastError'], '') ? '<details><summary>Show error</summary><pre>' + escapeHTML(pick(item, ['last_error', 'LastError'], '')) + '</pre></details>' : '') +
                    '</article>';
            }), 'No channels configured.'));

            var runtimes = toArray(snapshot.runtimes);
            setText('runtimeCountTag', String(runtimes.length));
            setHTML('runtimeList', renderList(runtimes.map(function(item) {
                return '<article class="item">' +
                    '<div class="item-head">' +
                        '<div><h4 class="item-title">' + escapeHTML(pick(item, ['agent'], '--')) + '</h4>' +
                        '<div class="item-subtitle">' + escapeHTML(pick(item, ['workspace'], '--')) + '</div></div>' +
                        tag('hits ' + pick(item, ['hits'], 0), 'neutral') +
                    '</div>' +
                    '<div class="meta">' +
                        '<span>Created: ' + formatTime(pick(item, ['created_at'], '')) + '</span>' +
                        '<span>Last used: ' + formatTime(pick(item, ['last_used_at'], '')) + '</span>' +
                        '<span>Sessions: ' + escapeHTML(String(pick(item, ['session_count'], 0))) + '</span>' +
                    '</div>' +
                    '<div class="subtle">Path: ' + escapeHTML(pick(item, ['workspace_path'], '--')) + '</div>' +
                    '</article>';
            }), 'No cached runtime instances yet.'));

            var recentEvents = toArray(snapshot.recent_events);
            setHTML('statusEventList', renderList(recentEvents.map(function(item) {
                return '<article class="item">' +
                    '<div class="item-head">' +
                        '<div><h4 class="item-title">' + escapeHTML(pick(item, ['type'], 'event')) + '</h4>' +
                        '<div class="item-subtitle">' + formatTime(pick(item, ['timestamp'], '')) + '</div></div>' +
                        tag(pick(item, ['session_id'], 'global') || 'global', 'neutral') +
                    '</div>' +
                    '<pre class="log-json">' + prettyJSON(pick(item, ['payload'], {})) + '</pre>' +
                    '</article>';
            }), 'No recent events.'));

            if (!providerResp.ok) {
                setHTML('providerList', renderError(providerResp.error));
                setText('providerCountTag', 'err');
                return;
            }

            var providers = toArray(providerResp.data);
            state.providers = providers;
            setText('providerCountTag', String(providers.length));
            setHTML('providerList', renderList(providers.map(function(item) {
                var health = pick(item, ['health'], {});
                return '<article class="item">' +
                    '<div class="item-head">' +
                        '<div><h4 class="item-title">' + escapeHTML(pick(item, ['name'], pick(item, ['id'], 'provider'))) + '</h4>' +
                        '<div class="item-subtitle">' + escapeHTML(pick(item, ['provider'], '--')) + ' / ' + escapeHTML(pick(item, ['default_model'], '--')) + '</div></div>' +
                        tag(pick(item, ['is_default'], false) ? 'Default' : 'Optional', pick(item, ['is_default'], false) ? 'ok' : 'neutral') +
                    '</div>' +
                    '<div class="meta">' +
                        '<span>Base URL: ' + escapeHTML(pick(item, ['base_url'], 'provider default')) + '</span>' +
                        '<span>Agents: ' + escapeHTML(String(pick(item, ['bound_agent_count'], 0))) + '</span>' +
                        '<span>Has key: ' + escapeHTML(String(!!pick(item, ['has_api_key'], false))) + '</span>' +
                    '</div>' +
                    '<div class="subtle">' + tag(pick(health, ['status'], 'unknown'), toneForProvider(item)) + ' ' + escapeHTML(pick(health, ['message'], '')) + '</div>' +
                    '</article>';
            }), 'No provider profiles found.'));
        }
`

const dashboardScriptBHTML = `
        function renderSessions(resp) {
            if (!resp.ok) {
                setHTML('sessionList', renderError(resp.error));
                setText('sessionCountTag', 'err');
                return;
            }
            state.sessions = toArray(resp.data);
            renderSessionList();
        }

        function renderSessionList() {
            var query = String(document.getElementById('sessionSearch').value || '').trim().toLowerCase();
            var items = state.sessions.filter(function(item) {
                if (!query) return true;
                var haystack = [
                    pick(item, ['title'], ''),
                    pick(item, ['agent'], ''),
                    pick(item, ['workspace'], ''),
                    pick(item, ['last_user_text'], ''),
                    pick(item, ['last_assistant_text'], '')
                ].join(' ').toLowerCase();
                return haystack.indexOf(query) >= 0;
            });

            setText('sessionCountTag', String(items.length));
            setHTML('sessionList', renderList(items.map(function(item) {
                var messages = toArray(pick(item, ['messages'], []));
                var preview = messages.slice(Math.max(messages.length - 4, 0));
                var transcript = preview.length ? preview.map(function(msg) {
                    return '<div class="item" style="padding:10px 0;border:none;background:transparent;">' +
                        '<div class="meta"><strong>' + escapeHTML(String(pick(msg, ['role'], 'assistant')).toUpperCase()) + '</strong></div>' +
                        '<div>' + escapeHTML(String(pick(msg, ['content'], '')).slice(0, 300)) + '</div>' +
                        '</div>';
                }).join('') : renderEmpty('No session messages stored yet.');
                return '<article class="item">' +
                    '<div class="item-head">' +
                        '<div><h4 class="item-title">' + escapeHTML(pick(item, ['title'], 'New session')) + '</h4>' +
                        '<div class="item-subtitle">' + escapeHTML(pick(item, ['agent'], '--')) + ' / ' + escapeHTML(pick(item, ['workspace'], '--')) + '</div></div>' +
                        tag(String(pick(item, ['message_count'], 0)) + ' msg', 'neutral') +
                    '</div>' +
                    '<div class="meta">' +
                        '<span>Created: ' + formatTime(pick(item, ['created_at'], '')) + '</span>' +
                        '<span>Updated: ' + formatTime(pick(item, ['updated_at'], '')) + '</span>' +
                        '<span>Queue: ' + escapeHTML(String(pick(item, ['queue_depth'], 0))) + '</span>' +
                        '<span>Presence: ' + escapeHTML(pick(item, ['presence'], 'idle')) + '</span>' +
                    '</div>' +
                    '<div class="stack">' +
                        '<div class="subtle"><strong>User:</strong> ' + escapeHTML(String(pick(item, ['last_user_text'], 'none')).slice(0, 180)) + '</div>' +
                        '<div class="subtle"><strong>Assistant:</strong> ' + escapeHTML(String(pick(item, ['last_assistant_text'], 'none')).slice(0, 180)) + '</div>' +
                    '</div>' +
                    '<details><summary>Show recent messages</summary>' + transcript + '</details>' +
                    '</article>';
            }), 'No matching sessions.'));
        }

        function renderLogGroup(resp, listID, countID, renderer, emptyText) {
            if (!resp.ok) {
                setHTML(listID, renderError(resp.error));
                setText(countID, 'err');
                return;
            }
            var items = toArray(resp.data);
            setText(countID, String(items.length));
            setHTML(listID, renderList(items.map(renderer), emptyText));
        }

        function renderLogs(auditResp, eventResp, toolResp, jobResp) {
            renderLogGroup(auditResp, 'auditList', 'auditCountTag', function(item) {
                return '<article class="item">' +
                    '<div class="item-head">' +
                        '<div><h4 class="item-title">' + escapeHTML(pick(item, ['action'], 'audit')) + '</h4>' +
                        '<div class="item-subtitle">' + formatTime(pick(item, ['timestamp'], '')) + '</div></div>' +
                        tag(pick(item, ['actor'], 'anonymous'), 'neutral') +
                    '</div>' +
                    '<div class="meta"><span>Role: ' + escapeHTML(pick(item, ['role'], '--')) + '</span><span>Target: ' + escapeHTML(pick(item, ['target'], '--')) + '</span></div>' +
                    (pick(item, ['meta'], null) ? '<details><summary>Show meta</summary><pre>' + prettyJSON(pick(item, ['meta'], {})) + '</pre></details>' : '') +
                    '</article>';
            }, 'No audit entries.');

            renderLogGroup(eventResp, 'eventList', 'eventCountTag', function(item) {
                return '<article class="item">' +
                    '<div class="item-head">' +
                        '<div><h4 class="item-title">' + escapeHTML(pick(item, ['type'], 'event')) + '</h4>' +
                        '<div class="item-subtitle">' + formatTime(pick(item, ['timestamp'], '')) + '</div></div>' +
                        tag(pick(item, ['session_id'], 'global') || 'global', 'neutral') +
                    '</div>' +
                    '<pre class="log-json">' + prettyJSON(pick(item, ['payload'], {})) + '</pre>' +
                    '</article>';
            }, 'No events.');

            renderLogGroup(toolResp, 'toolList', 'toolCountTag', function(item) {
                var error = pick(item, ['error'], '');
                return '<article class="item">' +
                    '<div class="item-head">' +
                        '<div><h4 class="item-title">' + escapeHTML(pick(item, ['tool_name'], 'tool')) + '</h4>' +
                        '<div class="item-subtitle">' + formatTime(pick(item, ['timestamp'], '')) + '</div></div>' +
                        tag(error ? 'Error' : 'OK', error ? 'error' : 'ok') +
                    '</div>' +
                    '<div class="meta">' +
                        '<span>Agent: ' + escapeHTML(pick(item, ['agent'], '--')) + '</span>' +
                        '<span>Workspace: ' + escapeHTML(pick(item, ['workspace'], '--')) + '</span>' +
                        '<span>Session: ' + escapeHTML(pick(item, ['session_id'], '--')) + '</span>' +
                    '</div>' +
                    '<details><summary>Show args and result</summary><pre>' + prettyJSON({ args: pick(item, ['args'], {}), result: pick(item, ['result'], ''), error: error }) + '</pre></details>' +
                    '</article>';
            }, 'No tool activity.');

            renderLogGroup(jobResp, 'jobList', 'jobCountTag', function(item) {
                return '<article class="item">' +
                    '<div class="item-head">' +
                        '<div><h4 class="item-title">' + escapeHTML(pick(item, ['summary'], pick(item, ['kind'], 'job'))) + '</h4>' +
                        '<div class="item-subtitle">' + escapeHTML(pick(item, ['kind'], 'job')) + '</div></div>' +
                        tag(pick(item, ['status'], 'unknown'), toneForJob(pick(item, ['status'], 'unknown'))) +
                    '</div>' +
                    '<div class="meta">' +
                        '<span>Created: ' + formatTime(pick(item, ['created_at'], '')) + '</span>' +
                        '<span>Started: ' + formatTime(pick(item, ['started_at'], '')) + '</span>' +
                        '<span>Done: ' + formatTime(pick(item, ['completed_at'], '')) + '</span>' +
                    '</div>' +
                    ((pick(item, ['error'], '') || pick(item, ['details'], null)) ? '<details><summary>Show details</summary><pre>' + prettyJSON({ error: pick(item, ['error'], ''), details: pick(item, ['details'], {}), payload: pick(item, ['payload'], {}) }) + '</pre></details>' : '') +
                    '</article>';
            }, 'No jobs.');
        }

        function renderSkills(skillResp, pluginResp, bindingResp) {
            if (skillResp.ok) {
                var skills = toArray(skillResp.data);
                setText('skillCountTag', String(skills.length));
                setHTML('skillList', renderList(skills.map(function(item) {
                    var permissions = toArray(pick(item, ['permissions', 'Permissions'], []));
                    return '<article class="item">' +
                        '<div class="item-head">' +
                            '<div><h4 class="item-title">' + escapeHTML(pick(item, ['name', 'Name'], 'skill')) + '</h4>' +
                            '<div class="item-subtitle">' + escapeHTML(pick(item, ['version', 'Version'], 'unknown')) + '</div></div>' +
                            tag(String(permissions.length) + ' perms', permissions.length ? 'warn' : 'neutral') +
                        '</div>' +
                        '<div class="subtle">' + escapeHTML(pick(item, ['description', 'Description'], '')) + '</div>' +
                        '<div class="meta"><span>Entrypoint: ' + escapeHTML(pick(item, ['entrypoint', 'Entrypoint'], '--')) + '</span><span>Registry: ' + escapeHTML(pick(item, ['registry', 'Registry'], '--')) + '</span></div>' +
                        (permissions.length ? '<details><summary>Show permissions</summary><pre>' + prettyJSON(permissions) + '</pre></details>' : '') +
                        '</article>';
                }), 'No skills loaded.'));
            } else {
                setHTML('skillList', renderError(skillResp.error));
                setText('skillCountTag', 'err');
            }

            if (pluginResp.ok) {
                var plugins = toArray(pluginResp.data);
                setText('pluginCountTag', String(plugins.length));
                setHTML('pluginList', renderList(plugins.map(function(item) {
                    var kinds = toArray(pick(item, ['kinds'], []));
                    return '<article class="item">' +
                        '<div class="item-head">' +
                            '<div><h4 class="item-title">' + escapeHTML(pick(item, ['name'], 'plugin')) + '</h4>' +
                            '<div class="item-subtitle">' + escapeHTML(pick(item, ['version'], 'unknown')) + '</div></div>' +
                            tag(pick(item, ['enabled'], false) ? 'Enabled' : 'Disabled', pick(item, ['enabled'], false) ? 'ok' : 'warn') +
                        '</div>' +
                        '<div class="subtle">' + escapeHTML(pick(item, ['description'], '')) + '</div>' +
                        '<div class="meta"><span>Kinds: ' + escapeHTML(kinds.join(', ') || '--') + '</span><span>Trust: ' + escapeHTML(pick(item, ['trust'], '--')) + '</span></div>' +
                        '</article>';
                }), 'No plugins available.'));
            } else {
                setHTML('pluginList', renderError(pluginResp.error));
                setText('pluginCountTag', 'err');
            }
`

const dashboardScriptCHTML = `
            if (bindingResp.ok) {
                var bindings = toArray(bindingResp.data);
                setText('bindingCountTag', String(bindings.length));
                setHTML('bindingList', renderList(bindings.map(function(item) {
                    var skills = toArray(pick(item, ['skills'], []));
                    return '<article class="item">' +
                        '<div class="item-head">' +
                            '<div><h4 class="item-title">' + escapeHTML(pick(item, ['name'], 'agent')) + '</h4>' +
                            '<div class="item-subtitle">' + escapeHTML(pick(item, ['provider_name'], '--')) + ' / ' + escapeHTML(pick(item, ['model'], '--')) + '</div></div>' +
                            tag(pick(item, ['active'], false) ? 'Active' : 'Idle', pick(item, ['active'], false) ? 'ok' : 'neutral') +
                        '</div>' +
                        '<div class="meta"><span>Provider ref: ' + escapeHTML(pick(item, ['resolved_provider_ref'], pick(item, ['provider_ref'], '--'))) + '</span><span>Permission: ' + escapeHTML(pick(item, ['permission_level'], '--')) + '</span><span>Skills: ' + escapeHTML(String(skills.length)) + '</span></div>' +
                        (skills.length ? '<details><summary>Show skill bindings</summary><pre>' + prettyJSON(skills) + '</pre></details>' : '') +
                        '</article>';
                }), 'No agent bindings found.'));
            } else {
                setHTML('bindingList', renderError(bindingResp.error));
                setText('bindingCountTag', 'err');
            }
        }

        function renderConfig(configResp, providerResp) {
            if (!configResp.ok) {
                setText('configJSON', configResp.error);
                setText('configSizeTag', 'err');
                setText('configProviderStatus', 'Error');
                setText('configModelStatus', 'Error');
                return;
            }

            var cfg = configResp.data || {};
            state.config = cfg;
            setHTML('configJSON', prettyJSON(cfg));
            setText('configSizeTag', String(Object.keys(cfg).length));

            var llm = cfg.llm || {};
            setText('configModelStatus', llm.model || 'not set');

            if (!providerResp.ok) {
                setText('configProviderStatus', 'Provider read failed');
                return;
            }

            var providers = toArray(providerResp.data);
            state.providers = providers;
            var selected = '';
            var options = providers.map(function(item) {
                var id = pick(item, ['id'], '');
                var name = pick(item, ['name'], id);
                var isDefault = !!pick(item, ['is_default'], false);
                if (isDefault) selected = id;
                return '<option value="' + escapeHTML(id) + '"' + (isDefault ? ' selected' : '') + '>' +
                    escapeHTML(name + ' (' + pick(item, ['provider'], '--') + ')') +
                    '</option>';
            });
            if (!options.length) options.push('<option value="">No providers</option>');
            setHTML('defaultProviderSelect', options.join(''));
            setText('configProviderStatus', selected || 'not set');
            document.getElementById('globalModelInput').value = llm.model || '';
        }

        async function loadStatusTab() {
            var res = await Promise.all([safeRequest('/control-plane'), safeRequest('/providers')]);
            renderStatus(res[0], res[1]);
        }

        async function loadSessionsTab() {
            renderSessions(await safeRequest('/sessions'));
        }

        async function loadLogsTab() {
            var res = await Promise.all([
                safeRequest('/audit'),
                safeRequest('/events?limit=40'),
                safeRequest('/tools/activity?limit=40'),
                safeRequest('/jobs')
            ]);
            renderLogs(res[0], res[1], res[2], res[3]);
        }

        async function loadSkillsTab() {
            var res = await Promise.all([
                safeRequest('/skills'),
                safeRequest('/plugins'),
                safeRequest('/agent-bindings')
            ]);
            renderSkills(res[0], res[1], res[2]);
        }

        async function loadConfigTab() {
            var res = await Promise.all([safeRequest('/config'), safeRequest('/providers')]);
            renderConfig(res[0], res[1]);
        }

        async function loadNodesTab() {
            var res = await safeRequest('/nodes');
            renderNodes(res);
        }

        async function loadChannelsTab() {
            var res = await safeRequest('/channels');
            renderChannels(res);
        }

        async function loadRemoteTab() {
            var res = await safeRequest('/remote/status');
            renderRemote(res);
        }

        function renderNodes(nodesResp) {
            if (!nodesResp.ok) {
                setHTML('nodesList', renderError(nodesResp.error));
                return;
            }
            var nodes = toArray(nodesResp.data);
            setText('nodeCount', String(nodes.length));
            setHTML('nodesList', nodes.length ? nodes.map(function(node) {
                var statusClass = pick(node, ['state'], 'offline') === 'online' ? 'ok' : 'warn';
                return '<div class="list-item">' +
                    '<div><span class="item-title">' + escapeHTML(pick(node, ['name'], 'Node')) + '</span>' +
                    '<span class="tag ' + statusClass + '">' + escapeHTML(pick(node, ['state'], 'offline')) + '</span></div>' +
                    '<div class="meta"><span>Type: ' + escapeHTML(pick(node, ['type'], '--')) + '</span>' +
                    '<span>Capabilities: ' + String(pick(node, ['capabilities'], []).length) + '</span></div></div>';
            }).join('') : '<p class="muted">No nodes connected.</p>');
        }

        function renderChannels(channelsResp) {
            if (!channelsResp.ok) {
                setHTML('channelsList', renderError(channelsResp.error));
                return;
            }
            var channels = toArray(channelsResp.data);
            setText('channelCount', String(channels.length));
            setHTML('channelsList', channels.length ? channels.map(function(ch) {
                var statusClass = pick(ch, ['running'], false) && pick(ch, ['healthy'], false) ? 'ok' : 'warn';
                return '<div class="list-item">' +
                    '<div><span class="item-title">' + escapeHTML(pick(ch, ['name'], 'Channel')) + '</span>' +
                    '<span class="tag ' + statusClass + '">' + (pick(ch, ['running'], false) ? 'Running' : 'Stopped') + '</span></div>' +
                    '<div class="meta"><span>Enabled: ' + escapeHTML(String(pick(ch, ['enabled'], false))) + '</span></div></div>';
            }).join('') : '<p class="muted">No channels configured.</p>');
        }

        function renderRemote(remoteResp) {
            if (!remoteResp.ok) {
                setText('remoteStatus', 'Error');
                return;
            }
            var data = remoteResp.data || {};
            setText('remoteStatus', String(pick(data, ['status'], 'idle')));
            var nodes = toArray(pick(data, ['nodes'], []));
            setText('remoteNodeCount', String(nodes.length));
            setHTML('remoteNodesList', nodes.length ? nodes.map(function(n) {
                return '<div class="list-item"><div><span class="item-title">' + escapeHTML(pick(n, ['name'], 'Node')) + '</span>' +
                    '<span class="tag ' + (pick(n, ['online'], false) ? 'ok' : 'warn') + '">' + (pick(n, ['online'], false) ? 'Online' : 'Offline') + '</span></div></div>';
            }).join('') : '<p class="muted">No remote nodes.</p>');
        }

        async function refreshCurrentTab() {
            showNotice('', '');
            switch (state.tab) {
                case 'sessions':
                    await loadSessionsTab();
                    return;
                case 'logs':
                    await loadLogsTab();
                    return;
                case 'skills':
                    await loadSkillsTab();
                    return;
                case 'config':
                    await loadConfigTab();
                    return;
                case 'nodes':
                    await loadNodesTab();
                    return;
                case 'channels':
                    await loadChannelsTab();
                    return;
                case 'remote':
                    await loadRemoteTab();
                    return;
                default:
                    await loadStatusTab();
                    return;
            }
        }

        function scheduleRefresh() {
            if (refreshHandle) clearInterval(refreshHandle);
            refreshHandle = null;
            if (!document.getElementById('autoRefresh').checked) return;
            refreshHandle = setInterval(function() {
                refreshCurrentTab();
            }, 15000);
        }

        async function saveDefaultProvider() {
            var select = document.getElementById('defaultProviderSelect');
            var providerRef = select ? String(select.value || '').trim() : '';
            if (!providerRef) {
                showNotice('error', 'No provider selected.');
                return;
            }
            var resp = await safeRequest('/providers/default', { method: 'POST', body: { provider_ref: providerRef } });
            if (!resp.ok) {
                document.getElementById('providerActionNote').textContent = resp.error;
                showNotice('error', 'Failed to switch default provider.');
                return;
            }
            document.getElementById('providerActionNote').textContent = 'Default provider set to ' + pick(resp.data, ['name', 'id'], providerRef) + '.';
            showNotice('success', 'Default provider updated.');
            await Promise.all([loadStatusTab(), loadConfigTab()]);
        }

        async function testDefaultProvider() {
            var select = document.getElementById('defaultProviderSelect');
            var providerRef = select ? String(select.value || '').trim() : '';
            if (!providerRef) {
                showNotice('error', 'No provider selected.');
                return;
            }
            var target = null;
            for (var i = 0; i < state.providers.length; i++) {
                if (pick(state.providers[i], ['id'], '') === providerRef) target = state.providers[i];
            }
            if (!target) {
                showNotice('error', 'Selected provider not found.');
                return;
            }
            var resp = await safeRequest('/providers/test', { method: 'POST', body: target });
            if (!resp.ok) {
                document.getElementById('providerActionNote').textContent = resp.error;
                showNotice('error', 'Provider test failed.');
                return;
            }
            document.getElementById('providerActionNote').textContent = pick(resp.data, ['message'], 'Provider test finished.');
            showNotice('success', 'Provider test finished.');
        }

        async function saveGlobalModel() {
            var model = String(document.getElementById('globalModelInput').value || '').trim();
            var resp = await safeRequest('/config', { method: 'POST', body: { llm: { model: model } } });
            if (!resp.ok) {
                document.getElementById('modelActionNote').textContent = resp.error;
                showNotice('error', 'Failed to save fallback model.');
                return;
            }
            document.getElementById('modelActionNote').textContent = 'Global fallback model saved as ' + (model || '(empty)') + '.';
            showNotice('success', 'Fallback model saved.');
            await Promise.all([loadStatusTab(), loadConfigTab()]);
        }

        function bindEvents() {
            var nav = document.querySelectorAll('.nav button');
            for (var i = 0; i < nav.length; i++) {
                nav[i].addEventListener('click', function() {
                    activateTab(this.getAttribute('data-tab'));
                    refreshCurrentTab();
                });
            }

            document.getElementById('refreshCurrent').addEventListener('click', function() {
                refreshCurrentTab();
            });
            document.getElementById('autoRefresh').addEventListener('change', scheduleRefresh);
            document.getElementById('sessionSearch').addEventListener('input', renderSessionList);
            document.getElementById('saveDefaultProvider').addEventListener('click', saveDefaultProvider);
            document.getElementById('testDefaultProvider').addEventListener('click', testDefaultProvider);
            document.getElementById('saveGlobalModel').addEventListener('click', saveGlobalModel);
            window.addEventListener('hashchange', function() {
                var tab = String(window.location.hash || '').replace('#', '');
                activateTab(tab || 'status');
                refreshCurrentTab();
            });
        }

        function init() {
            bindEvents();
            var initial = String(window.location.hash || '').replace('#', '');
            activateTab(initial || 'status');
            scheduleRefresh();
            refreshCurrentTab();
        }

        init();
    </script>
</body>
</html>
`

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.serveControlUIAsset(w, r) {
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML))
}

func (s *Server) serveControlUIAsset(w http.ResponseWriter, r *http.Request) bool {
	root, ok := s.resolveControlUIRoot()
	if !ok {
		return false
	}

	base := s.controlUIRouteBaseForPath(r.URL.Path)
	relURL := strings.TrimPrefix(r.URL.Path, base)
	if relURL == "" || relURL == "/" {
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
		return true
	}

	clean := path.Clean("/" + strings.TrimPrefix(relURL, "/"))
	assetPath, safe := joinWithinRoot(root, clean)
	if !safe {
		http.Error(w, "not found", http.StatusNotFound)
		return true
	}

	if info, err := os.Stat(assetPath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, assetPath)
		return true
	}

	// SPA fallback: let client-side routing handle unknown paths.
	http.ServeFile(w, r, filepath.Join(root, "index.html"))
	return true
}

func (s *Server) resolveControlUIRoot() (string, bool) {
	candidates := make([]string, 0, 5)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		candidates = append(candidates, value)
	}

	add(os.Getenv("ANYCLAW_CONTROL_UI_ROOT"))
	if s != nil && s.app != nil {
		add(config.ResolvePath(s.app.ConfigPath, s.app.Config.Gateway.ControlUI.Root))
		if s.app.ConfigPath != "" {
			add(filepath.Join(filepath.Dir(s.app.ConfigPath), "dist", "control-ui"))
		}
		if s.app.WorkingDir != "" {
			add(filepath.Join(s.app.WorkingDir, "dist", "control-ui"))
		}
	}
	if wd, err := os.Getwd(); err == nil {
		add(filepath.Join(wd, "dist", "control-ui"))
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		indexPath := filepath.Join(abs, "index.html")
		info, err := os.Stat(indexPath)
		if err != nil || info.IsDir() {
			continue
		}
		return abs, true
	}
	return "", false
}

func joinWithinRoot(root string, relCleanPath string) (string, bool) {
	target := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(relCleanPath, "/")))
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", false
	}
	if targetAbs == rootAbs {
		return targetAbs, true
	}
	sep := string(filepath.Separator)
	if strings.HasPrefix(targetAbs, rootAbs+sep) {
		return targetAbs, true
	}
	return "", false
}

func (s *Server) registerDashboardRoutes(mux *http.ServeMux) {
	for _, base := range s.controlUIRouteBases() {
		mux.HandleFunc(base, s.wrap(base, s.handleDashboard))
		mux.HandleFunc(base+"/", s.wrap(base+"/", s.handleDashboard))
	}
}

func (s *Server) controlUIBasePath() string {
	if s == nil || s.app == nil {
		return "/dashboard"
	}
	return normalizeControlUIBasePathValue(s.app.Config.Gateway.ControlUI.BasePath)
}

func (s *Server) controlUIRouteBases() []string {
	base := s.controlUIBasePath()
	bases := []string{base, "/dashboard", "/control"}
	uniq := make([]string, 0, len(bases))
	seen := make(map[string]struct{}, len(bases))
	for _, item := range bases {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		uniq = append(uniq, item)
	}
	return uniq
}

func (s *Server) controlUIRouteBaseForPath(requestPath string) string {
	for _, base := range s.controlUIRouteBases() {
		if requestPath == base || strings.HasPrefix(requestPath, base+"/") {
			return base
		}
	}
	return s.controlUIBasePath()
}

func normalizeControlUIBasePathValue(raw string) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		return "/dashboard"
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	base = path.Clean(base)
	if base == "." || base == "/" {
		return "/dashboard"
	}
	return strings.TrimRight(base, "/")
}
