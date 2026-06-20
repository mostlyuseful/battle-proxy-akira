---
id: provider.error-classification
title: Provider error classification
status: active
priority: p1
domain: provider
slice: synthetic-model-aliases-and-routing-value
depends_on:
  - provider.openai-nonstream
tags:
  - provider
  - errors
  - routing
context_files:
  - docs/spec.md
---

# Provider error classification

## Goal

Classify upstream provider failures into retryable and non-retryable proxy errors.

## Acceptance Criteria

- [ ] Classify HTTP 429, 408, 502, 503, 504, connection reset, and timeouts as retryable where appropriate.
- [ ] Classify invalid request, no valid credential, policy denied, input too large, and unsupported modality as non-retryable.
- [ ] Map provider error payloads to internal error codes where possible.
- [ ] Expose retryability information to the router without leaking provider implementation details.
- [ ] Add tests for representative HTTP statuses and network errors.

## Context

The router needs consistent error semantics for fallback and future circuit breakers.

## Out of Scope

- Provider-specific subscription exhaustion parsing

## Notes

401/403 retryability should remain conservative until multiple credential support exists.
