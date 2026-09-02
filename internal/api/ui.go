package api

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

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
    .chat-lite { display: flex; flex-direction: column; gap: 8px; margin-top: 6px; }
    .bubble { max-width: 80%; padding: 8px 10px; border-radius: 12px; white-space: pre-wrap; }
    .bubble.user { align-self: flex-end; background: #dff1ff; }
    .bubble.assistant { align-self: flex-start; background: #ececec; }
    .bubble.system { align-self: center; background: #f6e7c8; }
    details.raw-json { margin-top: 10px; }
  </style>
</head>
<body>
  <h1>llm-proxy UI</h1>
  <div class="row">
    <label>Bearer token <input id="token" type="password" size="40" placeholder="sk-..."></label>
    <label><input id="remember-token" type="checkbox"> Remember token on this browser</label>
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
const rememberTokenEl = document.getElementById('remember-token');
const statusEl = document.getElementById('status');
const modelsEl = document.getElementById('models');
const logsEl = document.getElementById('logs');
const pollEl = document.getElementById('poll');
const tabLogsEl = document.getElementById('tab-logs');
const tabModelsEl = document.getElementById('tab-models');
const panelLogsEl = document.getElementById('panel-logs');
const panelModelsEl = document.getElementById('panel-models');
let pollTimer = null;
let modelsLoaded = false;
const rememberedTokenKey = 'llm_proxy_ui_token';
const rememberTokenFlagKey = 'llm_proxy_ui_remember_token';

function headers() {
  const token = tokenEl.value.trim();
  return token ? { Authorization: 'Bearer ' + token } : {};
}
function setStatus(msg) { statusEl.textContent = msg; }

function restoreRememberedToken() {
  const remember = localStorage.getItem(rememberTokenFlagKey) === 'true';
  rememberTokenEl.checked = remember;
  if (remember) {
    tokenEl.value = localStorage.getItem(rememberedTokenKey) || '';
  }
}

function persistRememberedToken() {
  if (rememberTokenEl.checked) {
    localStorage.setItem(rememberTokenFlagKey, 'true');
    localStorage.setItem(rememberedTokenKey, tokenEl.value);
  } else {
    localStorage.removeItem(rememberTokenFlagKey);
    localStorage.removeItem(rememberedTokenKey);
  }
}

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
  modelsLoaded = true;
  setStatus('models loaded');
}

let logCursor = '0';
let logsLoading = false;
let pendingLogsReset = false;

async function loadLogs(reset = false) {
  if (logsLoading) {
    pendingLogsReset ||= reset;
    return;
  }
  logsLoading = true;
  try {
    setStatus('loading logs...');
    const after = reset ? '0' : logCursor;
    const res = await fetch('/ui/api/logs?after=' + encodeURIComponent(after), { headers: headers() });
    const body = await res.json();
    if (!res.ok) throw new Error(body.error?.message || body.error || 'logs failed');
    if (reset || body.reset) {
      logsEl.innerHTML = '';
    }
    const lines = body.lines || [];
    for (const line of lines) {
      appendLogLine(line);
    }
    logCursor = body.cursor || '0';
    setStatus(body.enabled ? ('logs loaded (' + lines.length + ' new)') : 'logging disabled');
  } finally {
    logsLoading = false;
    if (pendingLogsReset) {
      pendingLogsReset = false;
      loadLogs(true).catch(err => setStatus(err.message));
    }
  }
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

  const chatLite = renderChatLite(record);
  if (chatLite) {
    const title = document.createElement('h4');
    title.textContent = 'Chat-lite';
    detail.appendChild(title);
    detail.appendChild(chatLite);
  }

  const rawDetails = document.createElement('details');
  rawDetails.className = 'raw-json';
  const rawSummary = document.createElement('summary');
  rawSummary.textContent = 'Raw JSON';
  rawDetails.appendChild(rawSummary);
  const rawPre = document.createElement('pre');
  rawPre.textContent = JSON.stringify(record, null, 2);
  rawDetails.appendChild(rawPre);
  detail.appendChild(rawDetails);
  details.appendChild(detail);
  logsEl.appendChild(details);
}

function renderChatLite(record) {
  const transcript = record.transcript;
  if (!transcript || typeof transcript !== 'object') {
    return null;
  }

  const root = document.createElement('div');
  root.className = 'chat-lite';
  const request = transcript.request;
  if (request && Array.isArray(request.messages)) {
    for (const message of request.messages) {
      const bubble = renderMessageBubble(message.role || 'user', message.content);
      if (bubble) root.appendChild(bubble);
    }
  }

  const attempts = Array.isArray(transcript.attempts) ? transcript.attempts : [];
  const streamed = [];
  for (const attempt of attempts) {
    if (Array.isArray(attempt.stream)) {
      for (const chunk of attempt.stream) {
        const text = extractStreamChunkText(chunk);
        if (text) streamed.push(text);
      }
    }
    if (attempt.response) {
      const responseText = extractResponseText(attempt.response);
      if (responseText) {
        const bubble = renderMessageBubble('assistant', responseText);
        if (bubble) root.appendChild(bubble);
      }
    }
  }
  if (streamed.length > 0) {
    const bubble = renderMessageBubble('assistant', streamed.join(''));
    if (bubble) root.appendChild(bubble);
  }

  return root.childNodes.length > 0 ? root : null;
}

function renderMessageBubble(role, content) {
  const text = normalizeContentText(content);
  if (!text) return null;
  const div = document.createElement('div');
  const safeRole = ['user', 'assistant', 'system'].includes(role) ? role : 'assistant';
  div.className = 'bubble ' + safeRole;
  div.textContent = text;
  return div;
}

function normalizeContentText(content) {
  if (typeof content === 'string') return content;
  if (Array.isArray(content)) {
    return content.map(part => {
      if (typeof part === 'string') return part;
      if (part && typeof part.text === 'string') return part.text;
      if (part && part.type === 'output_text' && typeof part.text === 'string') return part.text;
      return '';
    }).filter(Boolean).join('\n');
  }
  if (content && typeof content.text === 'string') return content.text;
  return '';
}

function extractResponseText(response) {
  if (!response || typeof response !== 'object') return '';
  if (Array.isArray(response.choices)) {
    return response.choices.map(choice => normalizeContentText(choice?.message?.content)).filter(Boolean).join('\n');
  }
  if (Array.isArray(response.output)) {
    const parts = [];
    for (const item of response.output) {
      if (Array.isArray(item?.content)) {
        parts.push(normalizeContentText(item.content));
      }
    }
    return parts.filter(Boolean).join('\n');
  }
  return '';
}

function extractStreamChunkText(chunk) {
  if (!chunk || typeof chunk !== 'object') return '';
  if (Array.isArray(chunk.choices)) {
    return chunk.choices.map(choice => choice?.delta?.content || '').filter(Boolean).join('');
  }
  return '';
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
  if (!showLogs && !modelsLoaded) {
    loadModels().catch(err => setStatus(err.message));
  }
}

tabLogsEl.onclick = () => activateTab('logs');
tabModelsEl.onclick = () => activateTab('models');
rememberTokenEl.onchange = () => persistRememberedToken();
tokenEl.oninput = () => {
  if (rememberTokenEl.checked) persistRememberedToken();
};

document.getElementById('load-models').onclick = () => loadModels().catch(err => setStatus(err.message));
document.getElementById('load-logs').onclick = () => loadLogs(true).catch(err => setStatus(err.message));
pollEl.onchange = () => {
  clearInterval(pollTimer);
  if (pollEl.checked) {
    pollTimer = setInterval(() => loadLogs(false).catch(err => setStatus(err.message)), 3000);
  }
};
restoreRememberedToken();
pollEl.onchange();
</script>
</body>
</html>`

type logsResponse struct {
	Enabled bool     `json:"enabled"`
	Cursor  string   `json:"cursor,omitempty"`
	Reset   bool     `json:"reset,omitempty"`
	Lines   []string `json:"lines,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type logReadCursor struct {
	readerID    string
	generation  uint64
	offset      int64
	fingerprint string
}

type logFileReader struct {
	path       string
	readerID   string
	mu         sync.Mutex
	fileInfo   os.FileInfo
	generation uint64
}

const (
	logCursorVersion                = "v2"
	logCursorFingerprintWindowBytes = int64(64)
)

var errLegacyLogCursor = errors.New("legacy log cursor")

// RegisterUIRoutes wires a minimal built-in web UI and protected log viewer API.
func RegisterUIRoutes(mux *http.ServeMux, clientAuth Middleware, loggingCfg config.LoggingConfig) {
	if clientAuth == nil {
		clientAuth = identityMiddleware
	}
	logReader := newLogFileReader(loggingCfg.Path)
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
		cursor, lines, reset, err := logReader.readLinesSince(r.URL.Query().Get("after"), 200)
		if errors.Is(err, errLegacyLogCursor) {
			writeJSON(w, http.StatusConflict, logsResponse{Enabled: true, Error: "log cursor expired; click Load logs"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, logsResponse{Enabled: true, Error: "read logs failed"})
			return
		}
		writeJSON(w, http.StatusOK, logsResponse{Enabled: true, Cursor: cursor, Reset: reset, Lines: lines})
	})))
}

func newLogFileReader(path string) *logFileReader {
	reader := &logFileReader{path: path}
	var id [16]byte
	if _, err := rand.Read(id[:]); err == nil {
		reader.readerID = hex.EncodeToString(id[:])
	} else {
		// The identifier only distinguishes cursor generations; it is not a
		// secret. Keep the UI available even if the system RNG is unavailable.
		reader.readerID = strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return reader
}

func (r *logFileReader) readLinesSince(rawCursor string, maxLines int) (string, []string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, legacyCursor := parseLegacyLogCursor(rawCursor); legacyCursor {
		// A line-number cursor carries no file identity. Applying it after an
		// upgrade could silently skip records from a rotated or recreated file.
		return "", nil, false, errLegacyLogCursor
	}

	f, err := os.Open(r.path)
	if errors.Is(err, os.ErrNotExist) {
		// The logger creates its JSONL file lazily. If a file disappeared,
		// advance the generation so its cursor cannot apply to a recreated path.
		reset := rawCursor != "" && rawCursor != "0"
		r.markMissing()
		return r.encodeCursor(logReadCursor{}), []string{}, reset, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	defer f.Close()

	openedInfo, err := f.Stat()
	if err != nil {
		return "", nil, false, err
	}
	r.observe(openedInfo)

	cursor, validCursor := parseLogCursor(rawCursor)
	reset := false
	validateFingerprint := false
	switch {
	case rawCursor == "" || rawCursor == "0":
		cursor = r.currentCursor(0, "")
	case validCursor:
		if cursor.readerID != r.readerID || cursor.generation != r.generation {
			reset = true
			cursor = r.currentCursor(0, "")
		} else {
			validateFingerprint = cursor.offset > 0
		}
	default:
		reset = true
		cursor = r.currentCursor(0, "")
	}

	if cursor.offset > openedInfo.Size() {
		reset = true
		cursor = r.currentCursor(0, "")
		validateFingerprint = false
	} else if validateFingerprint {
		fingerprint, fingerprintErr := logFingerprintAt(f, cursor.offset)
		if errors.Is(fingerprintErr, io.EOF) || errors.Is(fingerprintErr, io.ErrUnexpectedEOF) {
			reset = true
			cursor = r.currentCursor(0, "")
		} else if fingerprintErr != nil {
			return "", nil, false, fingerprintErr
		} else if fingerprint != cursor.fingerprint {
			reset = true
			cursor = r.currentCursor(0, "")
		}
	}

	if _, err := f.Seek(cursor.offset, io.SeekStart); err != nil {
		return "", nil, false, err
	}
	if maxLines < 0 {
		maxLines = 0
	}
	lines := make([]string, 0, maxLines)
	offset := cursor.offset
	reader := bufio.NewReader(f)
	for len(lines) < maxLines {
		line, readErr := reader.ReadString('\n')
		if strings.HasSuffix(line, "\n") {
			offset += int64(len(line))
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			lines = append(lines, line)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return "", nil, false, readErr
		}
	}

	// Check the pathname after reading. If it was deleted or replaced while
	// this descriptor was open, discard the stale result and invalidate its
	// generation. An unlinked descriptor itself remains safe to read on Unix.
	pathInfo, err := os.Stat(r.path)
	if errors.Is(err, os.ErrNotExist) {
		r.markMissing()
		return r.encodeCursor(logReadCursor{}), []string{}, true, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	if !os.SameFile(openedInfo, pathInfo) {
		r.observe(pathInfo)
		return r.encodeCursor(logReadCursor{}), []string{}, true, nil
	}

	fingerprint, err := logFingerprintAt(f, offset)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		// The same file was truncated while it was being read.
		return r.encodeCursor(logReadCursor{}), []string{}, true, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	return r.encodeCursor(r.currentCursor(offset, fingerprint)), lines, reset, nil
}

func (r *logFileReader) observe(info os.FileInfo) {
	if r.fileInfo == nil {
		if r.generation == 0 {
			r.generation++
		}
		r.fileInfo = info
		return
	}
	if !os.SameFile(r.fileInfo, info) {
		r.generation++
		r.fileInfo = info
	}
}

func (r *logFileReader) markMissing() {
	if r.fileInfo != nil {
		r.generation++
		r.fileInfo = nil
	}
}

func (r *logFileReader) currentCursor(offset int64, fingerprint string) logReadCursor {
	return logReadCursor{
		readerID:    r.readerID,
		generation:  r.generation,
		offset:      offset,
		fingerprint: fingerprint,
	}
}

func (r *logFileReader) encodeCursor(cursor logReadCursor) string {
	cursor.readerID = r.readerID
	cursor.generation = r.generation
	if cursor.offset <= 0 {
		return fmt.Sprintf("%s:%s:%d:0", logCursorVersion, cursor.readerID, cursor.generation)
	}
	return fmt.Sprintf("%s:%s:%d:%d:%s", logCursorVersion, cursor.readerID, cursor.generation, cursor.offset, cursor.fingerprint)
}

func parseLogCursor(raw string) (logReadCursor, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) < 4 || parts[0] != logCursorVersion || parts[1] == "" {
		return logReadCursor{}, false
	}
	generation, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return logReadCursor{}, false
	}
	if len(parts) == 4 && parts[3] == "0" {
		return logReadCursor{readerID: parts[1], generation: generation}, true
	}
	if len(parts) != 5 {
		return logReadCursor{}, false
	}
	offset, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || offset <= 0 {
		return logReadCursor{}, false
	}
	fingerprint, err := hex.DecodeString(parts[4])
	if err != nil || len(fingerprint) != sha256.Size {
		return logReadCursor{}, false
	}
	return logReadCursor{
		readerID:    parts[1],
		generation:  generation,
		offset:      offset,
		fingerprint: hex.EncodeToString(fingerprint),
	}, true
}

func parseLegacyLogCursor(raw string) (int64, bool) {
	line, err := strconv.ParseInt(raw, 10, 64)
	return line, err == nil && line > 0
}

func logFingerprintAt(f *os.File, offset int64) (string, error) {
	if offset <= 0 {
		return "", nil
	}
	firstLen := min(offset, logCursorFingerprintWindowBytes)
	lastStart := max(int64(0), offset-logCursorFingerprintWindowBytes)
	data := make([]byte, firstLen+offset-lastStart)
	if _, err := f.ReadAt(data[:firstLen], 0); err != nil {
		return "", err
	}
	if _, err := f.ReadAt(data[firstLen:], lastStart); err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
