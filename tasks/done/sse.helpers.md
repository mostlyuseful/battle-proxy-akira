---
id: sse.helpers
title: SSE read and write helpers
status: done
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

- [x] Parse data: SSE lines from an upstream response stream.
- [x] Preserve [DONE] events.
- [x] Write data events as data: ... followed by a blank line.
- [x] Flush after each emitted event when the ResponseWriter supports flushing.
- [x] Add unit tests covering normal events, comments/blank lines, and [DONE].

## Context

Streaming should pass through OpenAI-compatible SSE where possible.

## Out of Scope

- Provider-specific event translation
- Responses API stream event mapping

## Notes

Keep this package independent of provider details.

Completed with `internal/sse` Reader/Event helpers, `[DONE]` preservation, data event writing/flushing, common SSE headers, and tests for normal events, comments/blank/non-data fields, multiline data, EOF final events, `[DONE]`, flushing, and headers.
