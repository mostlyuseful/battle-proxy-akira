---
id: router.availability-state
title: Provider and model availability state
status: done
priority: p2
domain: router
slice: access-token-auth-and-exhaustion-buffering
depends_on:
- provider.error-classification
- router.retry-candidates
tags:
- router
- availability
- state
context_files:
- docs/spec.md
---

# Provider and model availability state

## Goal

Track provider/model health, exhaustion windows, last error code, and failure counts.

## Acceptance Criteria

- [x] Implement an AvailabilityState structure for provider/model pairs.
- [x] Mark route candidates successful or failed from router outcomes.
- [x] Expose state checks so routing can skip unavailable candidates in later tasks.
- [x] Avoid global data races under concurrent requests.
- [x] Add tests for success, failure, and concurrent state access.

## Context

The spec defines AvailabilityState with Provider, Model, Healthy, ExhaustedUntil, LastErrorCode, and Failures.

## Out of Scope

- Admin model state endpoint
- Persistent state

## Notes

In-memory state is sufficient for MVP hardening.

Completed with mutex-protected in-memory `AvailabilityTracker` and `AvailabilityState` snapshots on `StaticRouter`. Route successes clear failure/exhaustion state, failures record provider/model, health, failure count, last classified error, and short exhaustion windows for rate-limit/exhausted errors. The router exposes `Availability`, `AvailabilityStates`, and `IsCandidateAvailable` for later candidate skipping. Tests cover success reset, classified/generic failures, exhaustion-window checks, and concurrent access.
