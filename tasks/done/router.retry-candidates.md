---
id: router.retry-candidates
title: Candidate fallback routing
status: done
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

- [x] Iterate candidates in configured order for first_available strategy.
- [x] Retry on retryable provider failures before a non-streaming response or before streaming starts.
- [x] Stop immediately for non-retryable failures such as invalid_request, policy_denied, unsupported_modality, or input too large.
- [x] Record retry count for later logging.
- [x] Add tests for retryable fallback, non-retryable stop, and all-candidates-failed behavior.

## Context

This builds on the safe streaming boundary from router.retry-before-stream.

## Out of Scope

- Circuit breaker/backoff
- Least-cost routing

## Notes

Keep request mutation between candidates minimal and explicit.

Completed with non-streaming Chat Completions fallback across router-provided candidates in order, retrying only provider-classified retryable failures before writing a response, stopping on non-retryable errors, preserving minimal request mutation via `RouteCandidate.ProviderRequest`, logging retry count/final candidate, and tests for retryable fallback, non-retryable stop, and all-candidates-failed behavior. Streaming pre-start retry remains covered by `router.retry-before-stream`.
