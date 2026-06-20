---
id: auth.token-cache
title: Expiry-aware token cache
status: done
priority: p2
domain: auth
slice: access-token-auth-and-exhaustion-buffering
depends_on:
- auth.access-token-sources
tags:
- auth
- cache
- tokens
context_files:
- docs/spec.md
---

# Expiry-aware token cache

## Goal

Cache access tokens until their expiry window requires refresh.

## Acceptance Criteria

- [x] Cache tokens returned by access-token sources with expires_at metadata.
- [x] Refresh tokens before expiry using refresh_before_seconds.
- [x] Handle sources without expiry conservatively according to documented behavior.
- [x] Ensure concurrent token requests do not stampede command execution.
- [x] Add tests using a fake clock or controllable time source.

## Context

Command-based access-token helpers may be expensive and should not run for every request.

## Out of Scope

- OAuth device flow

## Notes

Keep token values out of logs and errors.

Completed with expiry-aware caching for command access-token sources. Tokens with valid `expires_at` are cached until the configured refresh window, no-expiry outputs are treated as uncached one-shot values, command refresh is mutex-guarded to prevent concurrent stampedes, and tests cover fake-clock refresh timing, no-expiry behavior, and concurrent callers sharing one command execution.
