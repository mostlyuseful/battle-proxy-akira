---
id: api.responses-stream
title: Responses API streaming translation
status: done
priority: p3
domain: api
slice: responses-api-compatibility
depends_on:
- api.responses-nonstream
- sse.helpers
tags:
- api
- responses-api
- streaming
context_files:
- docs/spec.md
---

# Responses API streaming translation

## Goal

Support stream=true on /v1/responses with SSE output translated from provider events where needed.

## Acceptance Criteria

- [x] Detect stream=true on Responses requests.
- [x] Set SSE response headers and flush each event.
- [x] Translate provider stream events into a compatible Responses SSE shape for supported text output.
- [x] Handle pre-stream errors as JSON and mid-stream errors according to the project streaming policy.
- [x] Add integration tests for streaming Responses behavior.

## Context

Responses streaming is useful for future clients but should not destabilize Chat Completions streaming.

## Out of Scope

- Full Responses event taxonomy

## Notes

Document any intentionally unsupported Responses event types.

Implemented across `internal/sse`, `internal/openai/responses_stream.go`, and `internal/api/responses.go`. Added `sse.WriteTypedEvent` for named `event:`+`data:` SSE events. Added `ResponsesStreamTranslator` plus a `ChatCompletionChunk` parser: provider events (which carry raw Chat Completions chunk JSON) are parsed to extract `choices[0].delta.content`, finish reason, and optional usage, then translated into the Codex-CLI-compatible Responses lifecycle sequence: `response.created` → `response.output_item.added` → `response.content_part.added` → `response.output_text.delta`* → `response.output_text.done` → `response.content_part.done` → `response.output_item.done` → `response.completed`. The Responses handler now routes `stream: true` through `streamResponsesRequest`, which mirrors the chat streaming policy: retryable pre-stream errors fall back to the next candidate (JSON error if none), the SSE stream is never fallback-switched after the first event, and mid-stream failures emit an `error` SSE event then close (no closing lifecycle events). Response/item IDs are derived from the request ID; the requested model alias is preserved. Tests cover end-to-end translation (fake and real upstream provider), pre-stream JSON errors, retryable fallback, mid-stream error event, empty-stream lifecycle, usage propagation, and translator unit behavior (ordering, idempotent opening, garbage-chunk skipping, length→incomplete). Intentionally unsupported: tool/refusal/file/reasoning/function-call Responses event types are not emitted; only text-output deltas are translated.
