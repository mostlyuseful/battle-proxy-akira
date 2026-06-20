---
id: api.chat-stream
title: Streaming POST /v1/chat/completions
status: done
priority: p0
domain: api
slice: streaming-mvp
depends_on:
- provider.openai-stream
- api.chat-nonstream
tags:
- api
- chat-completions
- streaming
context_files:
- docs/spec.md
---

# Streaming POST /v1/chat/completions

## Goal

Support stream=true Chat Completions requests with OpenAI-compatible SSE output.

## Acceptance Criteria

- [x] Detect stream=true on incoming chat requests.
- [x] Set appropriate SSE response headers.
- [x] Forward upstream SSE data events to the client and flush each event.
- [x] Preserve the final [DONE] marker.
- [x] Return OpenAI-style JSON errors if failure occurs before streaming starts.
- [x] Add integration tests using a fake streaming upstream.

## Context

This completes the core MVP path for streaming clients.

## Out of Scope

- Fallback across candidates
- Responses streaming

## Notes

Once response bytes are sent, do not attempt to switch providers silently.

Completed by branching the existing Chat Completions handler on `stream:true`, resolving the first route candidate, calling provider `Stream`, returning OpenAI-style JSON for pre-stream failures, setting SSE headers after stream setup succeeds, forwarding each raw event payload with flushes via `internal/sse`, preserving `[DONE]`, and adding fake-upstream integration tests for streaming, pre-stream failure, and existing auth/error behavior.
