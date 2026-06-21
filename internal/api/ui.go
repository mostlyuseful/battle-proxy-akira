package api

import (
	"bufio"
	"net/http"
	"os"
	"strings"

	"battle-proxy-akira/internal/config"
)

const uiHTML = `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>llm-proxy UI</title>
  <style>
    body { font-family: sans-serif; margin: 16px; }
    input, button { font: inherit; }
    pre { background: #111; color: #eee; padding: 12px; overflow: auto; max-height: 60vh; }
    table { border-collapse: collapse; width: 100%; }
    th, td { border: 1px solid #ccc; padding: 6px; text-align: left; }
    .row { display: flex; gap: 12px; align-items: center; margin-bottom: 12px; flex-wrap: wrap; }
    .panel { margin-top: 20px; }
    .tabs { display: flex; gap: 8px; margin-top: 16px; }
    .tab-button { padding: 6px 10px; border: 1px solid #ccc; background: #f5f5f5; cursor: pointer; }
    .tab-button.active { background: #ddd; font-weight: bold; }
    .tab-panel { display: none; margin-top: 16px; }
    .tab-panel.active { display: block; }
    .log-list { display: flex; flex-direction: column; gap: 10px; }
    details.log-card { border: 1px solid #ccc; border-radius: 6px; padding: 8px 10px; }
    details.log-card summary { cursor: pointer; }
    .log-summary { display: flex; gap: 12px; flex-wrap: wrap; }
    .log-summary span { white-space: nowrap; }
    .log-detail { margin-top: 10px; }
    .log-detail h4 { margin: 10px 0 6px; }
  </style>
</head>
<body>
  <h1>llm-proxy UI</h1>
  <div class="row">
    <label>Bearer token <input id="token" type="password" size="40" placeholder="sk-..."></label>
    <button id="load-models">Load models</button>
    <button id="load-logs">Load logs</button>
    <label><input id="poll" type="checkbox" checked> poll logs</label>
    <span id="status"></span>
  </div>

  <div class="tabs">
    <button id="tab-logs" class="tab-button active" type="button">Logs</button>
    <button id="tab-models" class="tab-button" type="button">Models</button>
  </div>

  <div id="panel-logs" class="tab-panel active">
    <div class="panel">
      <h2>Logs</h2>
      <div id="logs" class="log-list"></div>
    </div>
  </div>

  <div id="panel-models" class="tab-panel">
    <div class="panel">
      <h2>Models</h2>
      <table>
        <thead><tr><th>ID</th><th>Owner</th></tr></thead>
        <tbody id="models"></tbody>
      </table>
    </div>
  </div>

<script>
const tokenEl = document.getElementById('token');
const statusEl = document.getElementById('status');
const modelsEl = document.getElementById('models');
const logsEl = document.getElementById('logs');
const pollEl = document.getElementById('poll');
const tabLogsEl = document.getElementById('tab-logs');
const tabModelsEl = document.getElementById('tab-models');
const panelLogsEl = document.getElementById('panel-logs');
const panelModelsEl = document.getElementById('panel-models');
let pollTimer = null;

function headers() {
  const token = tokenEl.value.trim();
  return token ? { Authorization: 'Bearer ' + token } : {};
}
function setStatus(msg) { statusEl.textContent = msg; }

async function loadModels() {
  setStatus('loading models...');
  const res = await fetch('/v1/models', { headers: headers() });
  const body = await res.json();
  if (!res.ok) throw new Error(body.error?.message || 'models failed');
  modelsEl.innerHTML = '';
  for (const model of body.data || []) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td>' + escapeHTML(model.id) + '</td><td>' + escapeHTML(model.owned_by) + '</td>';
    modelsEl.appendChild(tr);
  }
  setStatus('models loaded');
}

let logCursor = 0;

async function loadLogs(reset = false) {
  setStatus('loading logs...');
  const after = reset ? 0 : logCursor;
  const res = await fetch('/ui/api/logs?after=' + encodeURIComponent(after), { headers: headers() });
  const body = await res.json();
  if (!res.ok) throw new Error(body.error?.message || body.error || 'logs failed');
  if (reset) {
    logsEl.innerHTML = '';
  }
  const lines = body.lines || [];
  for (const line of lines) {
    appendLogLine(line);
  }
  logCursor = body.cursor || 0;
  setStatus(body.enabled ? ('logs loaded (' + lines.length + ' new)') : 'logging disabled');
}

function escapeHTML(s) {
  return String(s ?? '').replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
}

function appendLogLine(line) {
  let record;
  try {
    record = JSON.parse(line);
  } catch {
    const pre = document.createElement('pre');
    pre.textContent = line;
    logsEl.appendChild(pre);
    return;
  }

  const details = document.createElement('details');
  details.className = 'log-card';
  const summary = document.createElement('summary');
  summary.innerHTML = renderSummary(record);
  details.appendChild(summary);

  const detail = document.createElement('div');
  detail.className = 'log-detail';
  if (record.transcript) {
    const title = document.createElement('h4');
    title.textContent = 'Transcript';
    detail.appendChild(title);
    const transcriptPre = document.createElement('pre');
    transcriptPre.textContent = JSON.stringify(record.transcript, null, 2);
    detail.appendChild(transcriptPre);
  }
  const rawTitle = document.createElement('h4');
  rawTitle.textContent = 'Raw JSON';
  detail.appendChild(rawTitle);
  const rawPre = document.createElement('pre');
  rawPre.textContent = JSON.stringify(record, null, 2);
  detail.appendChild(rawPre);
  details.appendChild(detail);
  logsEl.appendChild(details);
}

function renderSummary(record) {
  const bits = [];
  bits.push('<span><strong>' + escapeHTML(record.ts || '') + '</strong></span>');
  bits.push('<span>' + escapeHTML(record.endpoint || '') + '</span>');
  bits.push('<span>' + escapeHTML(record.requested_model || '') + '</span>');
  if (record.resolved_provider || record.resolved_model) {
    bits.push('<span>' + escapeHTML((record.resolved_provider || '') + ':' + (record.resolved_model || '')) + '</span>');
  }
  bits.push('<span>status=' + escapeHTML(record.status ?? '') + '</span>');
  bits.push('<span>latency=' + escapeHTML(record.latency_ms ?? '') + 'ms</span>');
  bits.push('<span>request=' + escapeHTML(record.request_id || '') + '</span>');
  if (record.session_id) {
    bits.push('<span>session=' + escapeHTML(record.session_id) + '</span>');
  }
  return '<div class="log-summary">' + bits.join('') + '</div>';
}

function activateTab(name) {
  const showLogs = name === 'logs';
  tabLogsEl.classList.toggle('active', showLogs);
  tabModelsEl.classList.toggle('active', !showLogs);
  panelLogsEl.classList.toggle('active', showLogs);
  panelModelsEl.classList.toggle('active', !showLogs);
}

tabLogsEl.onclick = () => activateTab('logs');
tabModelsEl.onclick = () => activateTab('models');

document.getElementById('load-models').onclick = () => loadModels().catch(err => setStatus(err.message));
document.getElementById('load-logs').onclick = () => loadLogs(true).catch(err => setStatus(err.message));
pollEl.onchange = () => {
  clearInterval(pollTimer);
  if (pollEl.checked) {
    pollTimer = setInterval(() => loadLogs(false).catch(err => setStatus(err.message)), 3000);
  }
};
pollEl.onchange();
</script>
</body>
</html>`

type logsResponse struct {
	Enabled bool     `json:"enabled"`
	Cursor  int      `json:"cursor,omitempty"`
	Lines   []string `json:"lines,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// RegisterUIRoutes wires a minimal built-in web UI and protected log viewer API.
func RegisterUIRoutes(mux *http.ServeMux, clientAuth Middleware, loggingCfg config.LoggingConfig) {
	if clientAuth == nil {
		clientAuth = identityMiddleware
	}
	serveUI := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(uiHTML))
	}
	mux.HandleFunc("GET /", serveUI)
	mux.HandleFunc("GET /ui", serveUI)
	mux.Handle("GET /ui/api/logs", clientAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loggingCfg.Enabled || strings.TrimSpace(loggingCfg.Path) == "" {
			writeJSON(w, http.StatusOK, logsResponse{Enabled: false})
			return
		}
		after := parseNonNegativeInt(r.URL.Query().Get("after"))
		cursor, lines, err := readLogLinesSince(loggingCfg.Path, after, 200)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, logsResponse{Enabled: true, Error: "read logs failed"})
			return
		}
		writeJSON(w, http.StatusOK, logsResponse{Enabled: true, Cursor: cursor, Lines: lines})
	})))
}

func readLogLinesSince(path string, after int, maxLines int) (int, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lines := make([]string, 0, maxLines)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if lineNo <= after {
			continue
		}
		lines = append(lines, scanner.Text())
		if len(lines) >= maxLines {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, nil, err
	}
	for scanner.Scan() {
		lineNo++
	}
	if err := scanner.Err(); err != nil {
		return 0, nil, err
	}
	return lineNo, lines, nil
}

func parseNonNegativeInt(raw string) int {
	if raw == "" {
		return 0
	}
	value := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0
		}
		value = value*10 + int(r-'0')
	}
	if value < 0 {
		return 0
	}
	return value
}
