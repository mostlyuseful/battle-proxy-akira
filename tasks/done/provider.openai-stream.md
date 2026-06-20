---
id: provider.openai-stream
title: OpenAI-compatible streaming provider
status: done
priority: p0
domain: provider
slice: streaming-mvp
depends_on:
- sse.helpers
- provider.openai-nonstream
tags:
- provider
- streaming
- sse
context_files:
- docs/spec.md
---

# OpenAI-compatible streaming provider

## Goal

Stream Chat Completions from an OpenAI-compatible upstream provider using SSE.

## Acceptance Criteria

- [x] Send upstream chat requests with stream=true.
- [x] Read upstream SSE events incrementally.
- [x] Propagate client context cancellation to the upstream HTTP request.
- [x] Surface pre-stream upstream errors so API handlers can return JSON errors.
- [x] Expose streamed events through the provider Stream method or equivalent interface.
- [x] Add httptest coverage for streaming chunks and cancellation behavior where practical.

## Context

Fallback after the first streamed token is not safe and should not be introduced here.

## Out of Scope

- Retry/fallback
- Mid-stream error translation

## Notes

Avoid buffering the whole upstream response.

Completed by implementing `OpenAICompatibleProvider.Stream` with raw HTTP `stream=true` requests, incremental SSE parsing through `internal/sse`, context-cancelable upstream requests, pre-stream non-2xx status errors without leaking response bodies, raw payload `ir.Event` emission including `[DONE]`, and httptest coverage for chunks, pre-stream errors, and cancellation.
