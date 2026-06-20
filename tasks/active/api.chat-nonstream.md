---
id: api.chat-nonstream
title: Non-streaming POST /v1/chat/completions
status: active
priority: p0
domain: api
slice: first-useful-chat-completions-proxy
depends_on:
  - router.static-model
  - api.openai-errors
  - auth.client-bearer
tags:
  - api
  - chat-completions
  - integration
context_files:
  - docs/spec.md
---

# Non-streaming POST /v1/chat/completions

## Goal

Expose a working non-streaming OpenAI-compatible Chat Completions endpoint.

## Acceptance Criteria

- [ ] Register POST /v1/chat/completions.
- [ ] Parse and normalize incoming non-streaming chat requests.
- [ ] Route the request to a configured OpenAI-compatible provider.
- [ ] Return an OpenAI-compatible chat.completion JSON response.
- [ ] Return OpenAI-style errors for bad JSON, auth failure, unknown model, and upstream failure before response.
- [ ] Add an integration test using a fake upstream provider.

## Context

This should be the first end-to-end useful feature for text requests.

## Out of Scope

- Streaming behavior
- Synthetic model fallback
- Image inputs

## Notes

When upstream usage is unknown, follow the chosen compatibility behavior from the spec.
