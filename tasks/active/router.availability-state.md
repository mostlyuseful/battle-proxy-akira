---
id: router.availability-state
title: Provider and model availability state
status: active
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

- [ ] Implement an AvailabilityState structure for provider/model pairs.
- [ ] Mark route candidates successful or failed from router outcomes.
- [ ] Expose state checks so routing can skip unavailable candidates in later tasks.
- [ ] Avoid global data races under concurrent requests.
- [ ] Add tests for success, failure, and concurrent state access.

## Context

The spec defines AvailabilityState with Provider, Model, Healthy, ExhaustedUntil, LastErrorCode, and Failures.

## Out of Scope

- Admin model state endpoint
- Persistent state

## Notes

In-memory state is sufficient for MVP hardening.
