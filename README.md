![banner](banner.webp)

# Battle Proxy Akira

A small, dependency-light LLM proxy that exposes an OpenAI-compatible client API
and forwards requests to one or more upstream providers. It supports chat
completions and responses, streaming via SSE, text and image inputs, synthetic
fallback aliases, optional metadata-only request logging, and live config
reload on `SIGHUP`.

## Build and run

```bash
make build
./llm-proxy -config ./config.json

# or use defaults
./llm-proxy

# or run via Make
make run
```

### CLI flags

```text
-addr string     server listen address (overrides config)
-config string   path to JSON config file
-help            show usage information
-verbose         log informational and debug messages
```

Reload config without restarting: `kill -HUP <pid>`.

---

## Configuration file

The proxy reads a JSON file passed via `-config`. If `-config` is empty, the
proxy starts with built-in defaults (no upstream providers; health endpoints
only).

All fields are optional except where noted. Unknown fields are rejected.

### Top-level shape

```jsonc
{
  "server": { /* ServerConfig */ },
  "client_auth": { /* ClientAuthConfig */ },
  "providers": { /* map<string, ProviderConfig> */ },
  "synthetic_models": { /* map<string, SyntheticModelConfig> */ },
  "logging": { /* LoggingConfig */ }
}
```

### `server`

Controls the local HTTP server.

| Field                  | Type   | Default        | Notes                                            |
|------------------------|--------|----------------|--------------------------------------------------|
| `addr`                 | string | `127.0.0.1:8080` | Listen address. Override at runtime with `-addr`. |
| `read_timeout_seconds` | int    | `30`           | Non-negative.                                    |
| `write_timeout_seconds`| int    | `0`            | `0` keeps streaming-friendly behavior.           |
| `idle_timeout_seconds` | int    | `120`          | Non-negative.                                    |
| `max_body_bytes`       | int64  | `20971520`     | Must be positive. 20 MiB by default.             |

Example:

```json
{
  "server": {
    "addr": "0.0.0.0:8080",
    "read_timeout_seconds": 30,
    "write_timeout_seconds": 0,
    "idle_timeout_seconds": 120,
    "max_body_bytes": 20971520
  }
}
```

### `client_auth`

Authenticates incoming client requests.

| Field       | Type   | Required when              | Notes                                   |
|-------------|--------|----------------------------|-----------------------------------------|
| `mode`      | string | always (defaults to `none`)| `none`, `static_bearer`, `bearer_tokens` |
| `tokens_env`| string | for bearer modes           | Env var name holding allowed token(s). |
| `tokens_val`| string | optional for `static_bearer` | Hardcoded client bearer token(s). |

Modes:

- `none` — no client auth.
- `static_bearer` — accepts token(s) from `tokens_val`, or falls back to the env var.
- `bearer_tokens` — accepts any token from a comma/newline-separated list in the env var.

Example:

```json
{
  "client_auth": {
    "mode": "static_bearer",
    "tokens_val": "local-dev-token"
  }
}
```

### `providers`

Map of upstream provider name to `ProviderConfig`.

```jsonc
{
  "providers": {
    "openai_api": { /* ProviderConfig */ },
    "codex":      { /* ProviderConfig */ }
  }
}
```

#### `ProviderConfig`

| Field    | Type                              | Required | Notes                                        |
|----------|-----------------------------------|----------|----------------------------------------------|
| `type`   | string                            | yes      | Currently only `openai_compatible`.          |
| `base_url` | string                          | yes      | Absolute `http(s)://...` URL.                |
| `auth`   | `AuthConfig`                      | yes      | How to fetch the upstream token.             |
| `models` | map<string, `ModelConfig`>        | yes      | At least one entry.                          |

#### `ModelConfig`

| Field       | Type     | Required | Notes                                   |
|-------------|----------|----------|-----------------------------------------|
| `modalities`| string[] | yes      | Subset of `text` and/or `image`.        |

Example:

```jsonc
{
  "providers": {
    "openai_api": {
      "type": "openai_compatible",
      "base_url": "https://api.openai.com/v1",
      "auth": { "type": "bearer_env", "env": "OPENAI_API_KEY" },
      "models": {
        "gpt-5.2":      { "modalities": ["text", "image"] },
        "gpt-5-mini":   { "modalities": ["text"] }
      }
      // Or omit `models` and the proxy will discover them from GET /models.
    }
  }
}
```

### `auth` (provider token source)

How the proxy retrieves an upstream provider token. The proxy never logs
secrets or headers.

| Field                    | Type     | Notes                                                                                |
|--------------------------|----------|--------------------------------------------------------------------------------------|
| `type`                   | string   | `bearer_env`, `bearer_val`, `env_access_token`, `file_access_token`, `access_token_command`. |
| `env`                    | string   | Env var name (`bearer_env`, `env_access_token`).                                     |
| `value`                  | string   | Inline bearer token (`bearer_val`).                                                  |
| `file`                   | string   | File path (`file_access_token`).                                                     |
| `command`                | string[] | Command to run (`access_token_command`). Output: `{"access_token":"...", "expires_at":"RFC3339"}`. |
| `refresh_before_seconds` | int      | Optional refresh window for `access_token_command` when `expires_at` is present.     |

Examples:

```jsonc
// 1. API key from environment variable.
{ "type": "bearer_env", "env": "OPENAI_API_KEY" }

// 2. Inline API key in config.
{ "type": "bearer_val", "value": "sk-..." }

// 3. OAuth-style token from environment.
{ "type": "env_access_token", "env": "CODEX_ACCESS_TOKEN" }

// 4. OAuth-style token from a local file.
{ "type": "file_access_token", "file": "/etc/llm-proxy/codex-token" }

// 5. Token from a command (e.g. a refresh CLI). Caches until near expiry.
{
  "type": "access_token_command",
  "command": ["codex", "auth", "token", "--json"],
  "refresh_before_seconds": 120
}
```

If `access_token_command` returns `expires_at` (RFC3339), the token is cached
and re-fetched `refresh_before_seconds` before expiry. If no `expires_at` is
returned, the command is invoked on every request.

### `synthetic_models`

Exposes aliases that map to a fallback pool of provider:model candidates.

| Field        | Type     | Notes                                                |
|--------------|----------|------------------------------------------------------|
| `strategy`   | string   | `first_available` or `least_cost_available`.         |
| `expose`     | bool     | If true, alias appears in `GET /v1/models`.          |
| `candidates` | string[] | Ordered `provider:model` references.                 |

Example:

```json
{
  "synthetic_models": {
    "coding": {
      "strategy": "first_available",
      "expose": true,
      "candidates": [
        "openai_api:gpt-5.2",
        "openai_api:gpt-5-mini"
      ]
    }
  }
}
```

Resolution rules:

- The router skips candidates whose model lacks a required modality (e.g.
  image requests).
- Unavailable candidates (recent failures) are skipped.
- If all candidates are exhausted, the proxy returns `no_available_model`.

### `logging`

Optional request logging to a JSONL file. Token redaction is applied before
writes; logger failures do not fail requests.

| Field     | Type   | Required when           | Notes                                                |
|-----------|--------|-------------------------|------------------------------------------------------|
| `enabled` | bool   | always (default false)  | Master switch.                                       |
| `mode`    | string | when `enabled`          | `off`, `metadata_only`, or `invasive`.               |
| `path`    | string | when `enabled`          | Local JSONL file path. Created if missing.           |

Example:

```json
{
  "logging": {
    "enabled": true,
    "mode": "metadata_only",
    "path": "/var/log/llm-proxy/requests.jsonl"
  }
}
```

`metadata_only` logs request metadata only. `invasive` also logs the raw
request body plus per-attempt upstream response/stream chunks with baseline
secret redaction applied. For multi-request session stitching, send an optional
`X-Session-Id` header; it is logged alongside each `request_id`.

---

## Endpoints

| Method | Path                   | Description                                       |
|--------|------------------------|---------------------------------------------------|
| GET    | `/healthz`             | Liveness probe.                                   |
| GET    | `/readyz`              | Readiness probe.                                  |
| GET    | `/metrics`             | Runtime metrics in JSON (counts, latency p50/95/99). |
| GET    | `/v1/models`           | List configured models and exposed synthetic aliases. |
| POST   | `/v1/chat/completions` | OpenAI-compatible chat completions (text + image, streaming). |
| POST   | `/v1/responses`        | OpenAI-compatible responses (text + image, streaming). |

---

## Full examples

### Minimal: defaults only

No `-config` and no `-addr`:

```bash
./llm-proxy
# /healthz, /readyz, /metrics are available; chat endpoints return no_available_model
```

### Single OpenAI-compatible provider

```json
{
  "server": { "addr": "127.0.0.1:8080" },
  "providers": {
    "openai_api": {
      "type": "openai_compatible",
      "base_url": "https://api.openai.com/v1",
      "auth": { "type": "bearer_env", "env": "OPENAI_API_KEY" },
      "models": {
        "gpt-5.2":    { "modalities": ["text", "image"] },
        "gpt-5-mini": { "modalities": ["text"] }
      }
    }
  },
  "synthetic_models": {
    "coding": {
      "strategy": "first_available",
      "expose": true,
      "candidates": ["openai_api:gpt-5.2", "openai_api:gpt-5-mini"]
    }
  },
  "logging": {
    "enabled": true,
    "mode": "metadata_only",
    "path": "./llm-proxy.jsonl"
  }
}
```

```bash
OPENAI_API_KEY=sk-... ./llm-proxy -config ./config.json
```

### Two providers, OAuth-style token, image-aware routing

```json
{
  "server": { "addr": "0.0.0.0:8080", "max_body_bytes": 33554432 },
  "client_auth": { "mode": "bearer_tokens", "tokens_env": "CLIENT_TOKENS" },
  "providers": {
    "openai_api": {
      "type": "openai_compatible",
      "base_url": "https://api.openai.com/v1",
      "auth": { "type": "bearer_env", "env": "OPENAI_API_KEY" },
      "models": {
        "gpt-5.2": { "modalities": ["text", "image"] }
      }
    },
    "codex": {
      "type": "openai_compatible",
      "base_url": "https://api.openai.com/v1",
      "auth": {
        "type": "access_token_command",
        "command": ["codex", "auth", "token", "--json"],
        "refresh_before_seconds": 120
      },
      "models": {
        "codex-1": { "modalities": ["text"] }
      }
    }
  },
  "synthetic_models": {
    "default": {
      "strategy": "first_available",
      "expose": true,
      "candidates": ["openai_api:gpt-5.2", "codex:codex-1"]
    },
    "vision": {
      "strategy": "first_available",
      "expose": true,
      "candidates": ["openai_api:gpt-5.2"]
    }
  }
}
```

```bash
CLIENT_TOKENS="token-a,token-b" \
  OPENAI_API_KEY=sk-... \
  ./llm-proxy -config ./config.json -addr :80
```

---

## Verbose diagnostics

`-verbose` enables informational and debug messages via `log/slog` without
sacrificing safety: bearer tokens, upstream API keys, and `Authorization`
headers are never logged. Diagnostics cover:

- Config source (file path vs. defaults) and overridden server address.
- Runtime manager build and reload state.
- Provider base URL, configured models, and token source type (not values).
- Router decisions (direct model, synthetic candidate accepted/skipped/rejected).
- Provider call lifecycle (request sent, status class, response received).
- Request logger mode and per-record JSONL writes.
- Metrics collector initialization, request and error recordings, snapshots.

## Make targets

```text
make build   # go build ./cmd/llm-proxy
make run     # build and run ./llm-proxy
```
