---
id: api.responses-nonstream
title: Non-streaming POST /v1/responses
status: done
priority: p3
domain: api
slice: responses-api-compatibility
depends_on:
- openai.responses-types
- router.synthetic-models
tags:
- api
- responses-api
- compatibility
context_files:
- docs/spec.md
---

# Non-streaming POST /v1/responses

## Goal

Expose a basic non-streaming Responses API endpoint backed by the existing IR and provider routing.

## Acceptance Criteria

- [x] Register POST /v1/responses.
- [x] Normalize supported Responses requests to IR.
- [x] Route requests through existing router/provider logic.
- [x] Return a Responses-compatible non-streaming output shape for successful text responses.
- [x] Return OpenAI-style errors for unsupported request features.
- [x] Add integration tests with a fake provider.

## Context

Where possible, reuse Chat provider calls and translate at the edge.

## Out of Scope

- Tool calls
- Files
- Batch jobs

## Notes

Unsupported features should fail clearly rather than being silently ignored.

Implemented in `internal/api/responses.go`: `RegisterResponsesRoutes` registers `POST /v1/responses`, applies client auth, request ID propagation, body-size limits, and metadata logging like the chat handler. Requests are parsed via `openaiapi.ParseResponseRequest` and normalized to IR via `ResponseRequest.ToIR`; streaming requests (`stream: true`) are rejected with a clear `invalid_request` error (streaming Responses is a separate task). The IR request is routed through the existing router/provider logic with retryable pre-response fallback across candidates, and the successful provider response is rewritten to the requested model alias via `RouteCandidate.RewriteResponse` and emitted with `openaiapi.ResponseFromIR`. The endpoint is wired into `NewServer` via a new `WithResponsesRouter` option that defaults to the chat router. Tests (fake provider) cover text end-to-end, image input with logged image metadata, model-alias rewrite, streaming rejection, errors (bad JSON, missing model, upstream failure), no-router, client auth, request-ID propagation, and retryable fallback to a second candidate.
