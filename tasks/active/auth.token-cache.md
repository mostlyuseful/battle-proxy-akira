---
id: auth.token-cache
title: Expiry-aware token cache
status: active
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

- [ ] Cache tokens returned by access-token sources with expires_at metadata.
- [ ] Refresh tokens before expiry using refresh_before_seconds.
- [ ] Handle sources without expiry conservatively according to documented behavior.
- [ ] Ensure concurrent token requests do not stampede command execution.
- [ ] Add tests using a fake clock or controllable time source.

## Context

Command-based access-token helpers may be expensive and should not run for every request.

## Out of Scope

- OAuth device flow

## Notes

Keep token values out of logs and errors.
