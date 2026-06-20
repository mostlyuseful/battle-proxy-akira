---
id: api.chat-stream
title: Streaming POST /v1/chat/completions
status: active
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

- [ ] Detect stream=true on incoming chat requests.
- [ ] Set appropriate SSE response headers.
- [ ] Forward upstream SSE data events to the client and flush each event.
- [ ] Preserve the final [DONE] marker.
- [ ] Return OpenAI-style JSON errors if failure occurs before streaming starts.
- [ ] Add integration tests using a fake streaming upstream.

## Context

This completes the core MVP path for streaming clients.

## Out of Scope

- Fallback across candidates
- Responses streaming

## Notes

Once response bytes are sent, do not attempt to switch providers silently.
