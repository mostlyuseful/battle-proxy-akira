---
id: auth.client-bearer
title: Optional client bearer authentication
status: active
priority: p1
domain: auth
slice: bootable-proxy-foundation
depends_on:
  - config.json-loader
  - api.openai-errors
tags:
  - auth
  - security
  - api
context_files:
  - docs/spec.md
---

# Optional client bearer authentication

## Goal

Protect proxy endpoints with optional client bearer-token authentication.

## Acceptance Criteria

- [ ] Implement client auth modes none and static_bearer.
- [ ] Read static bearer tokens from the configured environment variable.
- [ ] Reject missing or invalid Authorization headers with OpenAI-style errors.
- [ ] Allow mode none for loopback/local development only as configured.
- [ ] Add tests for accepted, rejected, and disabled auth cases.

## Context

Client auth is distinct from upstream provider auth. Never log client bearer tokens.

## Out of Scope

- Hashed bearer tokens
- mTLS
- Per-client quotas

## Notes

Middleware should be reusable across API endpoints.
