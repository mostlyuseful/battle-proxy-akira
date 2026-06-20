---
id: router.retry-candidates
title: Candidate fallback routing
status: active
priority: p1
domain: router
slice: synthetic-model-aliases-and-routing-value
depends_on:
  - router.synthetic-models
  - router.retry-before-stream
tags:
  - router
  - fallback
  - retries
context_files:
  - docs/spec.md
---

# Candidate fallback routing

## Goal

Try the next synthetic model candidate when a retryable pre-response failure occurs.

## Acceptance Criteria

- [ ] Iterate candidates in configured order for first_available strategy.
- [ ] Retry on retryable provider failures before a non-streaming response or before streaming starts.
- [ ] Stop immediately for non-retryable failures such as invalid_request, policy_denied, unsupported_modality, or input too large.
- [ ] Record retry count for later logging.
- [ ] Add tests for retryable fallback, non-retryable stop, and all-candidates-failed behavior.

## Context

This builds on the safe streaming boundary from router.retry-before-stream.

## Out of Scope

- Circuit breaker/backoff
- Least-cost routing

## Notes

Keep request mutation between candidates minimal and explicit.
