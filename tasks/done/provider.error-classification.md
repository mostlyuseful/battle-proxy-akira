---
id: provider.error-classification
title: Provider error classification
status: done
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

- [x] Classify HTTP 429, 408, 502, 503, 504, connection reset, and timeouts as retryable where appropriate.
- [x] Classify invalid request, no valid credential, policy denied, input too large, and unsupported modality as non-retryable.
- [x] Map provider error payloads to internal error codes where possible.
- [x] Expose retryability information to the router without leaking provider implementation details.
- [x] Add tests for representative HTTP statuses and network errors.

## Context

The router needs consistent error semantics for fallback and future circuit breakers.

## Out of Scope

- Provider-specific subscription exhaustion parsing

## Notes

401/403 retryability should remain conservative until multiple credential support exists.

Completed with provider-neutral classified errors carrying stable code/status/retryability, HTTP status and OpenAI-style payload code/type mapping, retryable network timeout/connection reset detection, API mapping of classified provider failures to OpenAI-style proxy errors, and tests for representative retryable/non-retryable statuses, payload refinement, no body/token leaks, and network errors.
