---
id: router.circuit-breaker
title: Rate-limit and exhaustion circuit breaker
status: done
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

- [x] Parse Retry-After headers when available.
- [x] Apply exponential backoff when Retry-After is absent.
- [x] Mark exhausted candidates unavailable until the backoff expires.
- [x] Skip unavailable candidates during synthetic model routing.
- [x] Clear or reduce failure state after successful requests.
- [x] Add tests for Retry-After, exponential backoff, skip, and recovery behavior.

## Context

Fallback should buffer provider exhaustion without distributed coordination.

## Out of Scope

- Distributed rate-limit coordination

## Notes

Keep behavior conservative for authentication failures unless another credential is known to exist.

Completed with provider `Retry-After` parsing, retry/exhaustion backoff state, and synthetic candidate filtering. `Retry-After` delta-seconds and HTTP-date values are preserved on classified provider errors; rate-limit/exhaustion failures use that timestamp or fall back to exponential process-local backoff (30s base, 5m cap). Success resets a provider/model pair. Synthetic aliases skip unavailable candidates; direct/explicit routes report `no_available_model` while their configured candidate is still in an exhaustion window. Authentication failures do not open the circuit.
