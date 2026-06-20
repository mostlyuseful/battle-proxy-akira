---
id: api.responses-stream
title: Responses API streaming translation
status: active
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

- [ ] Detect stream=true on Responses requests.
- [ ] Set SSE response headers and flush each event.
- [ ] Translate provider stream events into a compatible Responses SSE shape for supported text output.
- [ ] Handle pre-stream errors as JSON and mid-stream errors according to the project streaming policy.
- [ ] Add integration tests for streaming Responses behavior.

## Context

Responses streaming is useful for future clients but should not destabilize Chat Completions streaming.

## Out of Scope

- Full Responses event taxonomy

## Notes

Document any intentionally unsupported Responses event types.
