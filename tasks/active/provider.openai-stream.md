---
id: provider.openai-stream
title: OpenAI-compatible streaming provider
status: active
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

- [ ] Send upstream chat requests with stream=true.
- [ ] Read upstream SSE events incrementally.
- [ ] Propagate client context cancellation to the upstream HTTP request.
- [ ] Surface pre-stream upstream errors so API handlers can return JSON errors.
- [ ] Expose streamed events through the provider Stream method or equivalent interface.
- [ ] Add httptest coverage for streaming chunks and cancellation behavior where practical.

## Context

Fallback after the first streamed token is not safe and should not be introduced here.

## Out of Scope

- Retry/fallback
- Mid-stream error translation

## Notes

Avoid buffering the whole upstream response.
