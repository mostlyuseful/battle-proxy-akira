---
id: sse.helpers
title: SSE read and write helpers
status: active
priority: p0
domain: sse
slice: streaming-mvp
depends_on:
  - project.bootstrap
tags:
  - sse
  - streaming
  - http
context_files:
  - docs/spec.md
---

# SSE read and write helpers

## Goal

Provide small helpers for reading and writing server-sent events compatible with OpenAI streams.

## Acceptance Criteria

- [ ] Parse data: SSE lines from an upstream response stream.
- [ ] Preserve [DONE] events.
- [ ] Write data events as data: ... followed by a blank line.
- [ ] Flush after each emitted event when the ResponseWriter supports flushing.
- [ ] Add unit tests covering normal events, comments/blank lines, and [DONE].

## Context

Streaming should pass through OpenAI-compatible SSE where possible.

## Out of Scope

- Provider-specific event translation
- Responses API stream event mapping

## Notes

Keep this package independent of provider details.
