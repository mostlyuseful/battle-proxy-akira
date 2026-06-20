---
id: api.request-ids
title: Request IDs through handlers, logs, and errors
status: active
priority: p1
domain: api
slice: metadata-logging-and-baseline-safety
depends_on:
  - logging.jsonl-metadata
tags:
  - api
  - logging
  - observability
context_files:
  - docs/spec.md
---

# Request IDs through handlers, logs, and errors

## Goal

Generate and propagate per-request IDs for correlation across responses and logs.

## Acceptance Criteria

- [ ] Generate a unique request ID when the client does not provide one.
- [ ] Accept or preserve a safe incoming request ID header if the implementation chooses to support it.
- [ ] Store the request ID in context for handlers, router, provider calls, and logging.
- [ ] Include the request ID in error responses or response headers as appropriate.
- [ ] Add tests verifying request ID propagation into logs and errors.

## Context

Request IDs are useful before adding deeper operational hardening.

## Out of Scope

- Distributed tracing

## Notes

Use stdlib crypto/rand or another dependency-free unique ID strategy.
