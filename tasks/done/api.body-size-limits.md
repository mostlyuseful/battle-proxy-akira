---
id: api.body-size-limits
title: Request body size limits
status: done
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

- [x] Apply server.max_body_bytes to relevant request handlers.
- [x] Return an OpenAI-style input-too-large or invalid_request error with HTTP 413 when exceeded.
- [x] Ensure oversized bodies are rejected before full JSON parsing where possible.
- [x] Add tests for accepted body, rejected body, and default limit behavior.

## Context

The spec includes max_body_bytes in server config and flags image data as sensitive/large.

## Out of Scope

- Per-client quotas
- Token counting

## Notes

Use http.MaxBytesReader or equivalent stdlib approach.

Completed with `WithServerConfig` support for `server.max_body_bytes`, default API body limit from `config.DefaultMaxBodyBytes`, Chat Completions enforcement via `Content-Length` precheck and `http.MaxBytesReader`, OpenAI-style `input_too_large` 413 errors, metadata logging of rejected oversized requests, and tests for accepted bodies, configured rejection, and default-limit rejection.
