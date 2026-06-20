---
id: auth.bearer-env-source
title: Provider API key bearer env token source
status: active
priority: p0
domain: auth
slice: first-useful-chat-completions-proxy
depends_on:
  - config.json-loader
tags:
  - auth
  - providers
  - secrets
context_files:
  - docs/spec.md
---

# Provider API key bearer env token source

## Goal

Implement provider token retrieval for API-key providers using environment variables.

## Acceptance Criteria

- [ ] Define a TokenSource interface with Token(ctx) behavior.
- [ ] Implement bearer_env provider auth using the configured environment variable.
- [ ] Return a clear error when the env var is unset or empty.
- [ ] Ensure token values are never included in errors or logs.
- [ ] Add unit tests for present and missing token cases.

## Context

Provider auth is upstream-facing and separate from client auth.

## Out of Scope

- Access-token command/file/env sources
- Token caching

## Notes

The returned token should be usable as Authorization: Bearer <token>.
