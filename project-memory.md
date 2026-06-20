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
