---
id: auth.client-bearer
title: Optional client bearer authentication
status: done
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

- [x] Implement client auth modes none and static_bearer.
- [x] Read static bearer tokens from the configured environment variable.
- [x] Reject missing or invalid Authorization headers with OpenAI-style errors.
- [x] Allow mode none for loopback/local development only as configured.
- [x] Add tests for accepted, rejected, and disabled auth cases.

## Context

Client auth is distinct from upstream provider auth. Never log client bearer tokens.

## Out of Scope

- Hashed bearer tokens
- mTLS
- Per-client quotas

## Notes

Middleware should be reusable across API endpoints.

Completed with reusable `NewClientAuthMiddleware`/`StaticBearerAuth` API middleware, `none` identity mode, comma-separated env-token support for `static_bearer` and existing `bearer_tokens` alias, OpenAI-style rejection errors, no token echoing in errors, and tests for accepted/rejected/disabled auth plus `/v1/models` middleware integration.
