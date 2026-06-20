## Recommendation

Use **Go**. It fits the constraints better than Python/Node/Rust for this project: strong stdlib HTTP stack, easy streaming, static binaries, good concurrency, simple deployment, and fewer dependencies by default. Rust is viable, but implementing OpenAI-compatible streaming and JSON shape translation will be slower. Python/Node are faster to prototype, but harder to keep supply-chain-minimal.

The key design decision: build the proxy around a **small internal request/response IR**, not around OpenAI JSON directly. Then expose OpenAI-compatible endpoints on the client side and translate to provider-specific formats on the provider side.

OpenAI’s current docs confirm that Chat Completions remains an endpoint shape with `POST /chat/completions`, and the Responses API supports `stream: true` with server-sent events. The image input formats differ between Chat Completions and Responses: Chat Completions uses `image_url` content parts, while Responses uses `input_image` parts. ([OpenAI Platform][1])

---

# LLM Proxy Spec v0.1

## Goals

Build a local or server-deployed LLM proxy that:

1. Exposes an **OpenAI-compatible client API**.
2. Supports **streaming** via SSE.
3. Supports **text and image inputs**.
4. Forwards common sampling params such as `temperature`, `top_p`, `max_tokens`, `max_completion_tokens`, `stop`, `presence_penalty`, and `frequency_penalty`.
5. Supports provider adapters:

   * OpenAI-compatible API providers using API keys.
   * OpenAI/Codex-style subscription authentication using OAuth/access-token style credentials where officially available.
6. Supports **synthetic model names**, e.g. `coding`, mapped to fallback model pools.
7. Supports optional request-level logging, including full transcript and provider routing metadata.
8. Minimizes third-party dependencies.

## Non-goals for MVP

Do not implement full feature parity with every OpenAI endpoint initially.

Exclude for MVP:

* embeddings
* image generation
* audio
* batch jobs
* assistants/threads
* tool-call execution
* file upload proxying
* admin UI
* distributed rate-limit coordination

---

# Recommended API Surface

Expose these endpoints first:

```text
GET  /v1/models
POST /v1/chat/completions
POST /v1/responses        optional but recommended
GET  /healthz
GET  /readyz
```

For strict OpenAI compatibility, prioritize `/v1/chat/completions`. For your own clients and future models, also support `/v1/responses` internally or as a public endpoint. OpenAI’s docs show both current Chat Completions and Responses APIs; Responses is more natural for multimodal and future request shapes, while many clients still expect Chat Completions. ([OpenAI Platform][1])

---

# Architecture

```text
Client
  |
  | OpenAI-compatible HTTP/SSE
  v
Proxy API Layer
  |
  | parse + validate + normalize
  v
Internal Request IR
  |
  | resolve synthetic model
  | apply policy
  | choose provider/model
  v
Router
  |
  +--> Provider: OpenAI-compatible API key
  |
  +--> Provider: OpenAI-compatible OAuth/access-token
  |
  +--> Provider: future Anthropic/Gemini/local
  |
  v
Stream/Response Translator
  |
  | OpenAI-compatible response/SSE
  v
Client
```

## Main packages

Use mostly stdlib:

```text
cmd/llm-proxy/        main package
internal/api/         HTTP handlers
internal/openai/      OpenAI-compatible request/response structs
internal/ir/          normalized internal request model
internal/router/      model resolution, fallback, retry
internal/provider/    provider interface and adapters
internal/auth/        API key and access-token sources
internal/config/      config loading
internal/logging/     request logging/redaction
internal/sse/         SSE encode/decode helpers
internal/store/       optional local log storage
```

Avoid framework dependencies. `net/http`, `httputil`, `encoding/json`, `context`, `sync`, `time`, `crypto`, and `log/slog` cover most of this.

One dependency may be worth allowing: a TOML/YAML parser. To stay stdlib-only, use JSON config. If you want comments and better operator ergonomics, use TOML with a carefully pinned dependency.

---

# Provider interface

Use a minimal provider abstraction:

```go
type Provider interface {
    Name() string
    Complete(ctx context.Context, req ir.Request) (*ir.Response, error)
    Stream(ctx context.Context, req ir.Request) (<-chan ir.Event, error)
    Models(ctx context.Context) ([]ir.Model, error)
    Health(ctx context.Context) error
}
```

Do not leak OpenAI structs into the provider interface. Normalize once at the edge.

## Internal request IR

```go
type Request struct {
    ID        string
    Model     string
    Messages  []Message
    Params    SamplingParams
    Stream    bool
    Metadata  map[string]string
    RawBody   json.RawMessage
}

type Message struct {
    Role    string
    Content []ContentPart
}

type ContentPart struct {
    Type     string // "text", "image_url", "input_image"
    Text     string
    ImageURL string // URL or data URL
    Detail   string // "low", "high", "auto"
}

type SamplingParams struct {
    Temperature        *float64
    TopP               *float64
    MaxTokens          *int
    MaxCompletionTokens *int
    Stop               []string
    PresencePenalty    *float64
    FrequencyPenalty   *float64
    Seed               *int
}
```

Keep unknown params in `RawBody` or an `Extra map[string]json.RawMessage` so they can be forwarded to OpenAI-compatible providers without the proxy needing to understand every field.

---

# Streaming

Use **SSE pass-through where possible**.

For OpenAI-compatible providers:

1. Send upstream request with `stream: true`.
2. Read upstream response line by line.
3. Validate it is SSE-like.
4. Optionally log chunks.
5. Forward `data: ...\n\n` unchanged unless model name or provider metadata must be rewritten.

OpenAI documents `stream: true` for model responses using server-sent events. Chat Completions also supports stream options such as including usage before the final `[DONE]` marker. ([OpenAI Platform][2])

## Streaming rules

The proxy should:

* flush after every SSE event
* propagate client cancellation to upstream via context
* enforce per-request timeout
* return OpenAI-compatible error JSON if upstream fails before streaming starts
* if upstream fails mid-stream, emit an error SSE event if possible, then close
* preserve `[DONE]` for Chat Completions compatibility

---

# Synthetic model names

Synthetic models are aliases with ordered fallback pools.

Example:

```json
{
  "models": {
    "coding": {
      "strategy": "first_available",
      "candidates": [
        "codex:gpt-5.1-codex-max",
        "openai:gpt-5.1-codex",
        "openai:gpt-5.2",
        "openai-compatible-local:qwen3-coder"
      ]
    },
    "cheap": {
      "strategy": "least_cost_available",
      "candidates": [
        "openai:gpt-5.2-mini",
        "openai-compatible-local:small"
      ]
    }
  }
}
```

## Resolution algorithm

For `model: "coding"`:

1. Expand to candidate list.
2. Filter candidates by modality:

   * text-only model cannot receive images
   * vision-capable model required if any image parts exist
3. Filter by health and circuit-breaker state.
4. Try first candidate.
5. On retryable failure, try next candidate.
6. Return response using requested model name by default:

   * client sees `"model": "coding"`
   * logs record actual provider/model

Retryable failures:

```text
429 rate_limit_exceeded
401/403 only if token marked exhausted and another credential exists
408/499/502/503/504
connection reset
provider-specific subscription exhaustion error
```

Non-retryable failures:

```text
400 invalid_request
401 no valid credential
403 policy denied
413 input too large
422 unsupported modality
```

---

# Provider auth

## API-key provider

Config:

```json
{
  "providers": {
    "openai": {
      "type": "openai_compatible",
      "base_url": "https://api.openai.com/v1",
      "auth": {
        "type": "bearer_env",
        "env": "OPENAI_API_KEY"
      }
    }
  }
}
```

Use `Authorization: Bearer <token>`.

OpenAI recommends keeping API keys private and not sharing them; the proxy should therefore support per-provider env vars, not config-file secrets. ([OpenAI Help Center][3])

## Codex / ChatGPT subscription-style auth

Treat this as an **access-token source**, not as a normal OpenAI Platform API key.

OpenAI’s Codex docs say Codex supports “Sign in with ChatGPT for subscription access” and “Sign in with an API key for usage-based access.” Codex CLI and IDE support both, while Codex cloud requires ChatGPT sign-in. The docs also say the browser login returns an access token to the CLI/IDE, and `codex login --with-access-token` can read a token from stdin. ([OpenAI Developers][4])

Spec this conservatively:

```json
{
  "providers": {
    "codex_sub": {
      "type": "openai_compatible",
      "base_url": "https://api.openai.com/v1",
      "auth": {
        "type": "access_token_command",
        "command": ["/usr/local/bin/get-codex-token"],
        "refresh_before_seconds": 300
      }
    }
  }
}
```

Do **not** make MVP depend on scraping `~/.codex/auth.json`. OpenAI notes Codex may cache login details locally in plaintext or OS credential storage, and access tokens should be treated like passwords. ([OpenAI Developers][4])

Better token source options:

```text
env_access_token        reads CODEX_ACCESS_TOKEN
file_access_token       reads root-owned file with 0600 permissions
command_access_token    invokes a local helper that returns JSON
device_flow             later, only if documented and stable enough
```

Command output:

```json
{
  "access_token": "eyJ...",
  "expires_at": "2026-06-20T16:00:00Z"
}
```

Important caveat: the exact Codex subscription API surface may not be equivalent to the public Platform API. Keep the provider adapter isolated so this can be changed without affecting the client API.

---

# Request logging

Make full transcript logging explicitly opt-in.

Config:

```json
{
  "logging": {
    "enabled": true,
    "mode": "metadata_only",
    "full_transcript_header": "X-LLM-Log-Full-Transcript",
    "storage": {
      "type": "jsonl",
      "path": "/var/log/llm-proxy/requests.jsonl"
    },
    "redact": {
      "authorization": true,
      "api_keys": true,
      "image_data_urls": "hash_only"
    }
  }
}
```

Modes:

```text
off
metadata_only
full_transcript
full_transcript_per_request
```

Log record:

```json
{
  "ts": "2026-06-20T12:00:00Z",
  "request_id": "req_...",
  "client": "vscode",
  "requested_model": "coding",
  "resolved_provider": "codex_sub",
  "resolved_model": "gpt-5.1-codex-max",
  "stream": true,
  "input_modalities": ["text", "image"],
  "sampling": {
    "temperature": 0.2,
    "top_p": 1
  },
  "status": 200,
  "latency_ms": 4312,
  "retry_count": 1,
  "transcript": null
}
```

For full transcript mode, store:

```json
"messages": [...]
```

For images, do not log base64 data URLs by default. Store hash, byte length, MIME type, and maybe original URL.

---

# OpenAI-compatible request handling

## Chat Completions input

Accept:

```json
{
  "model": "coding",
  "messages": [
    {
      "role": "user",
      "content": "write a test"
    }
  ],
  "temperature": 0.2,
  "stream": true
}
```

Also accept multimodal content parts:

```json
{
  "model": "coding",
  "messages": [
    {
      "role": "user",
      "content": [
        {"type": "text", "text": "What is wrong with this UI?"},
        {
          "type": "image_url",
          "image_url": {
            "url": "data:image/png;base64,..."
          }
        }
      ]
    }
  ]
}
```

OpenAI’s image docs show both URL and base64 data URL inputs for Chat Completions, and multiple images can be included in one request. ([OpenAI Platform][5])

## Response output

For non-streaming, emit standard Chat Completions shape:

```json
{
  "id": "chatcmpl-proxy-...",
  "object": "chat.completion",
  "created": 1780000000,
  "model": "coding",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 0,
    "completion_tokens": 0,
    "total_tokens": 0
  }
}
```

When upstream usage is unknown, set usage to `null` or omit it depending on compatibility mode.

---

# Error format

Return OpenAI-style errors:

```json
{
  "error": {
    "message": "No available provider for synthetic model coding",
    "type": "proxy_routing_error",
    "param": "model",
    "code": "no_available_model"
  }
}
```

Suggested internal error codes:

```text
invalid_request
unsupported_modality
unknown_model
no_available_model
provider_auth_failed
provider_rate_limited
provider_exhausted
upstream_error
stream_interrupted
policy_denied
```

---

# Circuit breaking and exhaustion buffering

For subscription exhaustion buffering, implement provider/model state:

```go
type AvailabilityState struct {
    Provider     string
    Model        string
    Healthy      bool
    ExhaustedUntil *time.Time
    LastErrorCode string
    Failures      int
}
```

When a provider returns an exhaustion/rate-limit error:

1. Parse `Retry-After` if present.
2. Otherwise apply exponential backoff.
3. Mark candidate unavailable until backoff expiry.
4. Try next candidate if request is idempotent enough.

For streaming, fallback is only safe **before** the first upstream token is sent. After a stream has begun, do not silently switch providers.

---

# Config shape

Use JSON for stdlib-only config:

```json
{
  "server": {
    "addr": "127.0.0.1:8080",
    "read_timeout_seconds": 30,
    "write_timeout_seconds": 0,
    "idle_timeout_seconds": 120,
    "max_body_bytes": 20971520
  },
  "client_auth": {
    "mode": "bearer_tokens",
    "tokens_env": "LLM_PROXY_CLIENT_TOKENS"
  },
  "providers": {
    "openai_api": {
      "type": "openai_compatible",
      "base_url": "https://api.openai.com/v1",
      "auth": {
        "type": "bearer_env",
        "env": "OPENAI_API_KEY"
      },
      "models": {
        "gpt-5.2": {
          "modalities": ["text", "image"]
        }
      }
    },
    "codex_sub": {
      "type": "openai_compatible",
      "base_url": "https://api.openai.com/v1",
      "auth": {
        "type": "access_token_command",
        "command": ["/usr/local/bin/get-codex-token"],
        "refresh_before_seconds": 300
      },
      "models": {
        "gpt-5.1-codex-max": {
          "modalities": ["text"]
        }
      }
    }
  },
  "synthetic_models": {
    "coding": {
      "strategy": "first_available",
      "expose": true,
      "candidates": [
        "codex_sub:gpt-5.1-codex-max",
        "openai_api:gpt-5.2"
      ]
    }
  },
  "logging": {
    "enabled": true,
    "mode": "metadata_only",
    "path": "./llm-proxy.jsonl"
  }
}
```

---

# Security posture

## Supply chain

MVP dependency budget:

```text
0 required third-party runtime dependencies
0 generated SDKs
0 web framework dependencies
0 OpenAI SDK dependency
```

Use raw HTTP. The OpenAI-compatible API is simple enough that SDKs are not worth the supply-chain surface here.

## Secrets

Rules:

* never log `Authorization`
* never log upstream tokens
* never log full base64 images unless explicitly enabled
* support env var secrets
* support command-based secret retrieval
* optionally support OS keychain later, but that needs a dependency or platform-specific code

## Client auth

Even for localhost, require optional bearer auth:

```http
Authorization: Bearer local-dev-token
```

Modes:

```text
none              only for loopback dev
static_bearer     comma-separated env var
hashed_bearer     config stores SHA-256 hashes
mTLS              future
```

## Logging risk

Full transcript logging is sensitive. Add:

```text
X-LLM-Log-Full-Transcript: true
```

or require config-level allowlist by client token.

---

# Implementation plan

## Phase 1: MVP

Build:

```text
GET /healthz
GET /v1/models
POST /v1/chat/completions
OpenAI-compatible provider with API key
SSE streaming passthrough
synthetic model alias resolution
metadata-only JSONL logs
basic retries before stream start
```

No OAuth yet; define the auth interface but implement only `bearer_env`.

## Phase 2: Multimodal and transcript logging

Add:

```text
image_url and base64 data URL parsing
modality-based model filtering
full transcript logging
image hash logging
request-size limits
better OpenAI error compatibility
```

## Phase 3: Codex subscription/access-token adapter

Add:

```text
access_token_env
access_token_file
access_token_command
token cache with expiry
provider-specific exhaustion detection
per-provider circuit breaker
```

Keep this behind a feature flag because Codex subscription auth is more policy- and product-sensitive than normal API-key auth.

## Phase 4: Responses API

Add:

```text
POST /v1/responses
Responses-to-IR normalization
IR-to-Chat fallback where possible
Responses SSE translation
```

## Phase 5: Operational hardening

Add:

```text
structured metrics endpoint
request IDs
config reload
graceful shutdown
per-client quotas
admin model state endpoint
unit/integration test suite
```

---

# Suggested first code skeleton

```text
cmd/llm-proxy/main.go
internal/api/server.go
internal/api/chat.go
internal/api/models.go
internal/ir/types.go
internal/openai/chat_types.go
internal/openai/translate.go
internal/provider/provider.go
internal/provider/openai_compatible.go
internal/auth/token_source.go
internal/router/router.go
internal/sse/sse.go
internal/logging/jsonl.go
internal/config/config.go
```

Core interfaces:

```go
type TokenSource interface {
    Token(ctx context.Context) (string, error)
}

type Router interface {
    Resolve(ctx context.Context, req ir.Request) ([]RouteCandidate, error)
    MarkFailure(candidate RouteCandidate, err error)
    MarkSuccess(candidate RouteCandidate)
}

type Logger interface {
    LogRequest(ctx context.Context, rec RequestLogRecord) error
}
```

---

# Design choices I would lock now

1. **Go is the right default.**
2. **Use stdlib HTTP, not a web framework.**
3. **Do not import provider SDKs.**
4. **Use an internal IR.**
5. **Support OpenAI Chat Completions first.**
6. **Treat Codex subscription auth as a token-source adapter, not a hardcoded OAuth hack.**
7. **Never fallback after first streamed token.**
8. **Make full transcript logging opt-in per request or per client.**
9. **Represent synthetic models as routing policies, not fake providers.**
10. **Use JSON config initially to preserve stdlib-only builds.**

[1]: https://platform.openai.com/docs/api-reference/chat/create "Chat | OpenAI API Reference"
[2]: https://platform.openai.com/docs/api-reference/responses/create "Create a model response | OpenAI API Reference"
[3]: https://help.openai.com/en/articles/5112595-best-practices-for-api-key-safety?utm_source=chatgpt.com "Best Practices for API Key Safety"
[4]: https://developers.openai.com/codex/auth "Authentication – Codex | OpenAI Developers"
[5]: https://platform.openai.com/docs/guides/images-vision "Images and vision | OpenAI API"
