---
id: router.circuit-breaker
title: Rate-limit and exhaustion circuit breaker
status: active
priority: p2
domain: router
slice: access-token-auth-and-exhaustion-buffering
depends_on:
  - router.availability-state
tags:
  - router
  - circuit-breaker
  - rate-limits
context_files:
  - docs/spec.md
---

# Rate-limit and exhaustion circuit breaker

## Goal

Temporarily skip provider/model candidates after rate-limit or exhaustion failures.

## Acceptance Criteria

- [ ] Parse Retry-After headers when available.
- [ ] Apply exponential backoff when Retry-After is absent.
- [ ] Mark exhausted candidates unavailable until the backoff expires.
- [ ] Skip unavailable candidates during synthetic model routing.
- [ ] Clear or reduce failure state after successful requests.
- [ ] Add tests for Retry-After, exponential backoff, skip, and recovery behavior.

## Context

Fallback should buffer provider exhaustion without distributed coordination.

## Out of Scope

- Distributed rate-limit coordination

## Notes

Keep behavior conservative for authentication failures unless another credential is known to exist.
