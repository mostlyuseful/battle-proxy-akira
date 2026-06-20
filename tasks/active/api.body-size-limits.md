---
id: api.body-size-limits
title: Request body size limits
status: active
priority: p1
domain: api
slice: multimodal-support
depends_on:
  - config.json-loader
  - openai.chat-image-parts
tags:
  - api
  - security
  - limits
context_files:
  - docs/spec.md
---

# Request body size limits

## Goal

Enforce configured maximum request body sizes to protect the proxy, especially with image data URLs.

## Acceptance Criteria

- [ ] Apply server.max_body_bytes to relevant request handlers.
- [ ] Return an OpenAI-style input-too-large or invalid_request error with HTTP 413 when exceeded.
- [ ] Ensure oversized bodies are rejected before full JSON parsing where possible.
- [ ] Add tests for accepted body, rejected body, and default limit behavior.

## Context

The spec includes max_body_bytes in server config and flags image data as sensitive/large.

## Out of Scope

- Per-client quotas
- Token counting

## Notes

Use http.MaxBytesReader or equivalent stdlib approach.
