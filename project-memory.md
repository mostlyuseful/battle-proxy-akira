# Project Memory

## Decisions

### Go module path default

- Context: `project.bootstrap` needed to initialize a Go module, but the spec does not define a public repository/import path and the git repo has no remote configured.
- Decision: use the local module path `battle-proxy-akira` for now.
- Rejected alternatives: a guessed hosted path such as `github.com/.../battle-proxy-akira`, because it would encode an unconfirmed remote location.
- Affected area: Go module/import paths for the initial server skeleton; can be changed later before publishing if a canonical remote path is chosen.

### OpenAI-compatible error status defaults

- Context: `api.openai-errors` required suitable HTTP status mappings, but the spec only enumerates internal codes and example JSON, not exact statuses/types.
- Decision: map invalid requests to 400, unknown models to 404, unsupported modality to 422, no available model/provider exhaustion to 503, upstream/stream interruption/provider auth failures to 502, rate limits to 429, and policy denial to 403. Use `proxy_routing_error` for routing/model lookup failures, `proxy_upstream_error` for provider/upstream failures, `policy_denied` for policy failures, and `invalid_request_error` otherwise.
- Rejected alternatives: returning 400 for unknown models/no available providers, because distinguishing missing models and temporarily unavailable routing is more useful to clients and operations.
- Affected area: API error compatibility and future handler/provider error reporting.

### Config loader defaults and strictness

- Context: `config.json-loader` needed sensible defaults and validation behavior, but the spec did not define what happens when no config path is supplied or whether unknown JSON fields are allowed.
- Decision: `config.Load("")` returns a local-development config that can boot health endpoints without providers, with client auth disabled, logging off, and server defaults from the spec. Explicit JSON is decoded strictly with unknown fields rejected, then validated.
- Rejected alternatives: requiring providers for default config, because that would prevent a fresh checkout from booting health endpoints; silently ignoring unknown fields, because typo detection is more useful for operator-managed config.
- Affected area: config loading/validation and future command-line startup behavior.

### Core IR response and event shape

- Context: `ir.core-types` defined Request/Message/ContentPart/SamplingParams in the spec, but Response, Event, Model, usage, and error details were under-specified.
- Decision: keep the IR provider-neutral and minimal: non-streaming `Response` has one assistant `Message`, finish reason, optional usage, metadata, and raw body; streaming `Event` carries a generic type, optional delta/text, finish reason, usage, error, metadata, and raw JSON; `Model` carries ID/provider/name/modalities/synthetic metadata. Request also includes `Extra map[string]json.RawMessage` alongside `RawBody` for unknown provider-compatible fields.
- Rejected alternatives: mirroring OpenAI Chat Completions choices/chunks directly in the IR, because that would leak edge API shape into provider interfaces and make future Responses/non-OpenAI adapters harder.
- Affected area: provider interface implementations, OpenAI translation, routing modality checks, and logging metadata.

### Chat Completions MVP parsing scope

- Context: `openai.chat-types` needed request/response structs and text normalization, while image content parts are explicitly a later task and OpenAI allows `stop` as either a string or array.
- Decision: parse Chat Completions with custom stdlib JSON types that preserve raw body and unknown top-level/message fields; support simple string `content` only for now and return a clear decode error for multimodal content arrays; normalize `stop` string or array into IR `[]string`; provide minimal non-streaming response structs and IR-to-Chat response conversion.
- Rejected alternatives: accepting and partially ignoring multimodal content arrays before modality support exists, because that could silently drop images; modeling every OpenAI request/response field now, because unknown fields can be preserved without expanding MVP scope.
- Affected area: OpenAI edge parsing, future image content parsing, provider request forwarding, and Chat Completions response rendering.

### OpenAI-compatible provider MVP behavior

- Context: `provider.openai-nonstream` required an MVP provider interface and non-streaming adapter, but streaming, retries, circuit breaking, and provider-specific error parsing are later tasks.
- Decision: define the full provider-neutral interface now (`Name`, `Complete`, `Stream`, `Models`, `Health`) but implement only non-streaming `Complete` for OpenAI-compatible providers; `Stream` returns `ErrStreamingUnsupported`, `Models` returns configured model metadata, and `Health` only checks context for now. `Complete` builds a fresh Chat Completions body from IR, forces `stream=false`, appends `/chat/completions` to the configured `base_url`, forwards preserved unknown top-level fields, and does not include upstream error bodies in returned status errors.
- Rejected alternatives: reusing the raw incoming request body as-is, because synthetic model resolution and stream forcing need controlled mutation; fetching live upstream `/models` in this task, because static config is enough for the first non-streaming provider and live discovery is out of scope.
- Affected area: provider adapter contract, router integration, future streaming provider work, and upstream error handling.

### Static direct model routing defaults

- Context: `router.static-model` needed deterministic direct model resolution and error behavior, but duplicate model names across providers and missing provider instances were not specified.
- Decision: support both direct model names and explicit `provider:model` notation. Direct model names are resolved by lexicographically sorted provider name for deterministic behavior when multiple providers configure the same model. A model absent from config returns `unknown_model`; a configured model whose provider instance is unavailable returns `no_available_model`. Static router `MarkSuccess`/`MarkFailure` are no-ops until fallback/circuit-breaker tasks.
- Rejected alternatives: treating duplicate direct model names as ambiguous errors, because a deterministic first-provider rule keeps the MVP simple; returning `unknown_model` when the provider instance is missing, because the model is configured but temporarily unroutable.
- Affected area: router integration, future synthetic alias expansion, API error mapping, and logging of requested vs provider model.

### Synthetic alias routing behavior

- Context: `router.synthetic-models` needed alias expansion, exposed model metadata, and response model rewriting, but alias/direct-name precedence, unavailable provider instances, and unsupported future strategies were not fully specified.
- Decision: explicit `provider:model` notation takes precedence; otherwise a configured synthetic alias takes precedence over direct model lookup. `first_available` aliases expand to configured candidate order, skipping candidates whose provider instance is not currently registered; if all configured candidates are skipped, return `no_available_model`. Exposed aliases appear as synthetic `ir.Model` records with provider `proxy`. `RouteCandidate` keeps requested alias plus actual provider/model and provides helpers to rewrite provider requests to the concrete model and responses back to the requested model.
- Rejected alternatives: returning unavailable candidates for later failures, because the router can only route to registered provider instances today; treating unsupported strategies as `unknown_model`, because the alias exists but is not routable by the current strategy implementation.
- Affected area: synthetic alias routing, `/v1/models` implementation, logging requested vs resolved model, and future fallback/strategy tasks.

### SSE helper parsing and writing defaults

- Context: `sse.helpers` needed OpenAI-compatible streaming helpers, but exact handling of comments, non-data fields, multiline data fields, and response headers was not specified.
- Decision: parse only `data:` fields for pass-through events, ignore comments/blank non-events/non-data fields, join multiple `data:` lines in one event with `\n`, preserve `[DONE]` as ordinary event data with an `IsDone` helper, and write multiline payloads as multiple `data:` lines followed by a blank line. Provide a small `SetHeaders` helper for common SSE response headers.
- Rejected alternatives: exposing provider/OpenAI-specific chunk structs in the SSE package, because this package should remain provider independent; dropping multiline payload support, because it is part of SSE conventions and cheap to support.
- Affected area: streaming provider implementation and API streaming handlers.

### OpenAI-compatible streaming provider event shape

- Context: `provider.openai-stream` needed to expose upstream SSE through the provider `Stream` method, but mid-stream error representation and chunk JSON translation are later concerns.
- Decision: `Stream` sends the same Chat Completions request path as `Complete` with `stream=true` and `Accept: text/event-stream`, validates non-2xx responses before returning the channel, then reads SSE incrementally in a goroutine. Each upstream `data:` payload becomes an `ir.Event` with `Type=message_delta`, `Text` set to the raw payload, and `Raw` set to the raw JSON bytes; `[DONE]` becomes `Type=done`. Context cancellation uses `http.NewRequestWithContext` and closes the stream goroutine without emitting a synthetic error event.
- Rejected alternatives: decoding OpenAI stream chunk JSON into structured deltas now, because the API layer can still pass through raw payloads and provider-specific event translation is out of scope; returning upstream error bodies on pre-stream status errors, because they may contain sensitive provider details.
- Affected area: streaming provider, API streaming handler, retry-before-first-token logic, and future stream translation/error handling.

### Models endpoint shape and auth hook

- Context: `api.models` needed an OpenAI-compatible `/v1/models` endpoint and consistent client auth, but the client-auth task is not implemented yet and the spec does not define model `owned_by` values.
- Decision: expose a small `ModelLister` interface consumed by the API layer, register `/v1/models` in `NewServer`, and apply an optional `Middleware` hook to client API routes so the later client-auth task can plug in consistently. Return `{object:"list", data:[...]}` with each model as `{id, object:"model", created:0, owned_by}`; direct models use their provider name as `owned_by`, synthetic/unknown-owner models use `proxy`.
- Rejected alternatives: hard-wiring the router concrete type into API handlers, because a small interface is easier to test and replace; blocking this endpoint on the future client-auth implementation, because the middleware hook preserves the integration point without expanding scope.
- Affected area: `/v1/models`, server construction options, future client-auth middleware, and API/router integration.

### Client bearer auth middleware behavior

- Context: `auth.client-bearer` required `none` and `static_bearer`, while the spec/config examples also use `bearer_tokens`, and token parsing/error redaction details were not fully specified.
- Decision: implement reusable API middleware that treats `static_bearer` and existing `bearer_tokens` as equivalent comma-separated env-var token modes. `none` is an identity middleware for local/dev configs. Missing or invalid client `Authorization: Bearer` headers return OpenAI-style `policy_denied` errors with `WWW-Authenticate`, and middleware construction errors avoid including token values or env var names.
- Rejected alternatives: supporting only `static_bearer`, because existing config validation/spec examples already allow `bearer_tokens`; logging or echoing env var names in auth setup errors, because names can reveal deployment secret conventions and are not necessary for runtime client errors.
- Affected area: API client-auth integration, server construction, `/v1/models`, and future chat/responses endpoints.

### Non-streaming Chat Completions endpoint behavior

- Context: `api.chat-nonstream` needed the first end-to-end chat path, while streaming behavior and synthetic fallback are later tasks and usage compatibility can be either omitted or null when unknown.
- Decision: register `/v1/chat/completions` for all servers and require a configured chat router for successful calls. The non-streaming path parses text-only Chat Completions into IR, resolves routes through the router, uses the first returned candidate only, rewrites provider requests/responses through route metadata, and returns OpenAI-compatible JSON with `usage` omitted when the provider does not return usage. Streaming is now handled by `api.chat-stream`.
- Rejected alternatives: silently treating `stream:true` as non-streaming, because that would violate client expectations; implementing candidate fallback here, because retry/fallback has dedicated router tasks; returning zero token usage when unknown, because omitting unknown usage is allowed by the spec and avoids misleading clients.
- Affected area: `/v1/chat/completions`, router/API integration, streaming handler follow-up, fallback follow-up, and usage compatibility.

### Streaming Chat Completions API behavior

- Context: `api.chat-stream` needed SSE output for `stream:true`, while fallback across candidates and provider-specific stream chunk translation are later tasks.
- Decision: use the same `/v1/chat/completions` handler to branch on `stream:true` after JSON parsing and route resolution. If provider `Stream` returns an error before headers are written, return OpenAI-style JSON. After a stream starts, set SSE headers and forward each upstream event's raw `Text` payload as `data: ...\n\n`, including `[DONE]`, flushing via the SSE helper; do not attempt fallback or model-name JSON rewriting mid-stream.
- Rejected alternatives: buffering the whole stream to inspect/rewrite chunks, because streaming should be incremental; silently falling back to later candidates after bytes are written, because the spec forbids switching providers after the first streamed token.
- Affected area: `/v1/chat/completions` streaming path, provider stream event contract, future retry-before-stream and stream translation tasks.

### Metadata JSONL logging defaults

- Context: `logging.jsonl-metadata` needed request IDs and logging behavior before the dedicated `api.request-ids` task exists, and the spec did not say whether logging failures should fail requests.
- Decision: add an `internal/logging` package with `off` no-op and `metadata_only` JSONL append modes, plus a small API `WithRequestLogger` hook. Chat handlers log metadata for completed handler paths and ignore logger errors so successful model responses are not failed by observability issues. The log request ID uses `X-Request-ID` when present and otherwise generates a temporary `req_<hex>` value until the request-ID task centralizes propagation.
- Rejected alternatives: failing successful requests when JSONL writes fail, because the task explicitly asks logging failures not to crash successful requests; waiting for `api.request-ids`, because request logging can use a local fallback and later be unified.
- Affected area: chat completion handlers, logging package, future request ID propagation, and later redaction/logging hardening tasks.

### Baseline redaction scope

- Context: `logging.redaction-baseline` needed no-secret guarantees and future redaction helpers, but full transcript/image redaction is out of scope and metadata logs should not include headers or request bodies.
- Decision: keep metadata records header/body-free and add baseline string/record redaction for common bearer-token and `sk-...` API-key patterns before JSONL serialization. API/provider error paths continue returning generic upstream messages/status codes without upstream bodies. Tests search combined response/log output for sentinel client bearer tokens and upstream API keys on representative failure paths.
- Rejected alternatives: deep transcript redaction now, because transcript logging is not implemented and has dedicated future work; trying to redact every possible secret format, because baseline helpers should cover current bearer/API-key risks without overfitting.
- Affected area: metadata JSONL logging, provider/API error handling, future structured logging and redaction hardening.

### Request ID propagation defaults

- Context: `api.request-ids` needed generated/preserved request IDs in handlers, logs, router/provider context, and error correlation, but did not mandate a header name or exact error-body shape.
- Decision: use `X-Request-ID` as the public correlation header, preserve incoming IDs only when they are short printable token-like values without whitespace or obvious secret patterns, generate `req_<128-bit hex>` IDs with `crypto/rand` otherwise, store the ID in request context, copy it into IR `ID` and `metadata.request_id`, and return it on all server responses via the response header. Error responses rely on the header rather than extending the OpenAI-compatible JSON error shape.
- Rejected alternatives: adding a non-standard `request_id` field to OpenAI error bodies, because clients may expect the strict error envelope; trusting arbitrary incoming header values, because unsafe values could leak secrets or control characters back to logs/headers.
- Affected area: API server middleware, chat handlers, request logging, router/provider call context, and future operational tracing.

### Chat image content part parsing scope

- Context: `openai.chat-image-parts` needed Chat Completions `image_url` content arrays, but image fetching/validation, Responses `input_image`, and transcript logging are later work.
- Decision: accept simple string content as before and accept content arrays containing only `text` and `image_url` parts. Require the OpenAI-shaped nested `image_url.url` string while otherwise accepting both ordinary URLs and data URLs without fetch/URL validation; preserve optional `detail` into IR. When converting IR back to OpenAI Chat requests, keep all-text messages as a string and emit multimodal arrays only when image parts are present.
- Rejected alternatives: validating URL schemes/base64 payloads now, because the task only needs request-shape validation; accepting `input_image` in Chat Completions, because that belongs to Responses/future adapters.
- Affected area: OpenAI edge parsing, IR normalization, OpenAI-compatible provider request serialization, future modality filtering and image redaction tasks.

### Request body size limit behavior

- Context: `api.body-size-limits` needed `server.max_body_bytes` enforcement for large image/data-URL payloads, but server config is currently exposed to the API layer via construction options rather than full runtime config wiring.
- Decision: default API servers to `config.DefaultMaxBodyBytes` and add `WithServerConfig` so `ServerConfig.MaxBodyBytes` controls chat request body limits. Chat handlers reject requests with oversized `Content-Length` before reading, otherwise wrap the body with `http.MaxBytesReader`, and return OpenAI-style `input_too_large` errors with HTTP 413 while logging the 413 metadata record.
- Rejected alternatives: returning generic 400 for oversized bodies, because 413 is specifically actionable for clients; disabling limits when no explicit option is passed, because the config default exists to protect fresh deployments.
- Affected area: API server options, Chat Completions body reading, OpenAI-style error mapping, and future endpoint request handling.

### Provider error classification defaults

- Context: `provider.error-classification` needed router-visible retry semantics without exposing provider internals, while subscription exhaustion and multi-credential handling are later work.
- Decision: add a provider-neutral `provider.Error` with stable code, HTTP status, provider name, and `Retryable` flag, plus helper functions for routers/API mapping. Treat HTTP 408/429/502/503/504, connection resets/refusals, and timeouts as retryable; keep 400/401/403/413/422 non-retryable; parse only OpenAI-style error `code`/`type` fields to refine internal codes and never include upstream error bodies or tokens in error strings. API responses now map classified provider failures to corresponding OpenAI-style proxy codes.
- Rejected alternatives: returning raw provider response bodies/messages for better diagnostics, because they can contain secrets or provider-specific implementation details; making 401/403 retryable, because multiple credential support/exhaustion detection is not implemented yet.
- Affected area: provider adapters, router retry/fallback decisions, API upstream error mapping, and future circuit breaker/exhaustion tasks.

### Modality filtering defaults

- Context: `router.modality-filtering` needed image requests to avoid text-only models, but missing modality metadata behavior was not specified and legacy tests/configs may omit modalities.
- Decision: use IR `InputModalities`/`HasImages` to require image capability only when normalized content contains image parts. Configured model modalities are authoritative; missing modality metadata is treated as text-only, so text requests can still route but image requests require explicit `image` capability. Direct model resolution continues scanning deterministic provider order and skips unsupported candidates; synthetic aliases filter unsupported candidates from the fallback list. If a configured model/alias exists but no candidate supports the required modalities, return `unsupported_modality` for OpenAI-compatible 422 mapping.
- Rejected alternatives: assuming missing modality metadata supports images, because that could send image payloads to text-only providers; treating unsupported image requests as `no_available_model`, because the model exists but lacks capability rather than availability.
- Affected area: router direct/synthetic candidate resolution, API routing error mapping, future fallback and capability-discovery tasks.

### Streaming retry safety boundary

- Context: `router.retry-before-stream` needed safe fallback for streaming requests while preserving the design rule that providers must never be switched after the first client-visible streamed token.
- Decision: keep candidate retry orchestration in the Chat Completions streaming API handler because it observes both route candidates and whether bytes have been sent to the client. Retry classified retryable provider failures from `Provider.Stream` before headers/data are written, and also retry retryable stream error events only if no event has been emitted. Once the first SSE event is forwarded, never try another candidate; emit an OpenAI-style `stream_interrupted` SSE error event and close instead. Log the final selected provider/model and the pre-stream retry count.
- Rejected alternatives: writing SSE headers before reading the first upstream event, because that would prevent JSON error responses and safe pre-stream fallback; hiding mid-stream failures by silently closing, because clients benefit from an explicit stream interruption marker when possible.
- Affected area: Chat Completions streaming handler, router candidate fallback semantics, provider retryability classification, and future circuit-breaker/fallback policy.

### Candidate fallback routing defaults

- Context: `router.retry-candidates` needed fallback across synthetic candidates for retryable pre-response failures, but the router interface only resolves candidates and does not execute provider calls.
- Decision: keep execution/fallback orchestration in the Chat Completions API handler, iterating router-provided candidates in order. For non-streaming calls, retry classified retryable provider errors before any response is written, stop immediately on non-retryable errors, and return the last candidate's classified error when all retryable candidates fail. For streaming, retain the existing pre-first-event retry boundary. The request is copied through `RouteCandidate.ProviderRequest` for each attempt so only the concrete provider model changes, and logs record the final attempted provider/model plus retry count.
- Rejected alternatives: expanding the router interface to execute provider requests, because that would mix routing with API response/stream state and duplicate the streaming safety boundary; returning a generic no-available-model when all candidates fail, because preserving the last classified provider code is more actionable for clients.
- Affected area: Chat Completions execution, synthetic candidate fallback, request logging retry counts, and future circuit-breaker/backoff tasks.

### Access-token source behavior

- Context: `auth.access-token-sources` needed subscription-style token sources for env, file, and command auth without depending on undocumented Codex local files.
- Decision: implement `env_access_token`, `file_access_token`, and `access_token_command` under the existing `TokenSource` interface. Env and file sources trim whitespace and reject empty values. Command sources execute the configured command directly, parse JSON with `access_token` and optional RFC3339 `expires_at`, reject malformed/empty/expired tokens, and do not cache tokens yet. Errors identify source type/path/env/command failure class but never include token values or command output.
- Rejected alternatives: caching command tokens until `expires_at`, because token cache/refresh behavior has separate future work; accepting plain-text command output, because the spec requires JSON and future expiry metadata.
- Affected area: provider auth construction, access-token based providers, future token caching/refresh and exhaustion handling.

### Access-token cache behavior

- Context: `auth.token-cache` needed command/access-token caching without leaking token values and while avoiding concurrent command stampedes.
- Decision: cache only command-source tokens that include valid `expires_at`; refresh when `expires_at <= now + refresh_before_seconds`. Sources or command outputs without expiry are handled conservatively as uncached one-shot tokens, so env/file sources and no-expiry command outputs are re-read/re-run. Command token refresh is guarded by a mutex so concurrent callers share one in-flight refresh and then use the cached token.
- Rejected alternatives: caching no-expiry tokens forever or for an arbitrary TTL, because without expiry metadata there is no safe refresh window; adding a background refresh goroutine, because on-demand refresh is simpler and enough for MVP.
- Affected area: access-token command auth, provider auth construction, future refresh/backoff/exhaustion handling.

### Image logging metadata defaults

- Context: `logging.image-hash` needed safe image metadata without raw base64 logging, while full transcript logging controls and configurable redaction policy are later work.
- Decision: extract image metadata from normalized IR when chat requests are parsed. For `data:*;base64,...` URLs, log only source, MIME type, decoded byte length, and SHA-256 of decoded bytes. For external image URLs, default to recording only that a URL image was present with `url_redacted=true`; no raw URL is stored until a redaction policy is introduced. Malformed data URLs are still treated as data URL inputs but omit hash/length rather than logging raw data.
- Rejected alternatives: logging external URLs by default, because URLs can contain sensitive query parameters; hashing the raw base64 string, because decoded bytes provide stable identity across base64 formatting variants.
- Affected area: metadata JSONL records, Chat Completions logging, future full transcript logging and redaction policy configuration.

### Availability state defaults

- Context: `router.availability-state` needed in-memory provider/model state, but backoff policy and admin state exposure are later work.
- Decision: add a mutex-protected `AvailabilityTracker` owned by `StaticRouter`. `MarkFailure` records provider/model, `Healthy=false`, increments failures, stores a classified provider error code (or `upstream_error` fallback), and applies a short in-memory exhaustion window for `provider_rate_limited`/`provider_exhausted`. `MarkSuccess` resets the pair to healthy and clears failures/error/exhaustion. Current routing does not skip candidates yet; `IsCandidateAvailable`, `Availability`, and `AvailabilityStates` expose snapshots for later routing/circuit-breaker tasks.
- Rejected alternatives: skipping candidates immediately on any failure, because no health probe/backoff policy exists yet and that could strand models indefinitely; persisting state, because MVP hardening only requires process-local state.
- Affected area: static router outcome recording, future circuit breaking/exhaustion buffering, and admin state reporting.
