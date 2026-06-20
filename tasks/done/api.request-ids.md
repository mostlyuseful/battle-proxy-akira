---
id: api.request-ids
title: Request IDs through handlers, logs, and errors
status: done
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

- [x] Generate a unique request ID when the client does not provide one.
- [x] Accept or preserve a safe incoming request ID header if the implementation chooses to support it.
- [x] Store the request ID in context for handlers, router, provider calls, and logging.
- [x] Include the request ID in error responses or response headers as appropriate.
- [x] Add tests verifying request ID propagation into logs and errors.

## Context

Request IDs are useful before adding deeper operational hardening.

## Out of Scope

- Distributed tracing

## Notes

Use stdlib crypto/rand or another dependency-free unique ID strategy.

Completed with `X-Request-ID` middleware, safe incoming ID preservation, `crypto/rand` generated `req_<hex>` IDs, context/IR metadata propagation through chat routing and provider calls, response header correlation for success/error responses, metadata log integration, and tests for generated/preserved IDs plus log/error propagation.
