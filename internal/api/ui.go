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

  <div class="panel">
    <h2>Models</h2>
    <table>
      <thead><tr><th>ID</th><th>Owner</th></tr></thead>
      <tbody id="models"></tbody>
    </table>
  </div>

  <div class="panel">
    <h2>Logs</h2>
    <pre id="logs"></pre>
  </div>

<script>
const tokenEl = document.getElementById('token');
const statusEl = document.getElementById('status');
const modelsEl = document.getElementById('models');
const logsEl = document.getElementById('logs');
const pollEl = document.getElementById('poll');
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
    logsEl.textContent = '';
  }
  const lines = body.lines || [];
  if (lines.length > 0) {
    logsEl.textContent += (logsEl.textContent ? '\n' : '') + lines.join('\n');
    logsEl.scrollTop = logsEl.scrollHeight;
  }
  logCursor = body.cursor || 0;
  setStatus(body.enabled ? ('logs loaded (' + lines.length + ' new)') : 'logging disabled');
}

function escapeHTML(s) {
  return String(s ?? '').replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
}

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
	mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(uiHTML))
	})
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
