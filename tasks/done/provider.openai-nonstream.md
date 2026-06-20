---
id: provider.openai-nonstream
title: OpenAI-compatible non-stream provider
status: done
priority: p0
domain: provider
slice: first-useful-chat-completions-proxy
depends_on:
- auth.bearer-env-source
- openai.chat-types
tags:
- provider
- openai-compatible
- http
context_files:
- docs/spec.md
---

# OpenAI-compatible non-stream provider

## Goal

Call an OpenAI-compatible upstream Chat Completions endpoint for non-streaming requests.

## Acceptance Criteria

- [x] Define the Provider interface with Name, Complete, Stream, Models, and Health methods or an MVP-compatible subset.
- [x] Implement non-streaming Complete using raw net/http requests.
- [x] Set Authorization: Bearer from the configured provider token source.
- [x] Post to the provider base_url /chat/completions path with a compatible JSON body.
- [x] Translate a successful upstream response into IR or an OpenAI-compatible response path usable by the API layer.
- [x] Add httptest coverage for request path, auth header, body forwarding, and response handling.

## Context

Use raw HTTP and no provider SDKs. Upstream structs may stay isolated in internal/openai.

## Out of Scope

- Streaming
- Retries
- Circuit breaking

## Notes

Be careful not to log upstream Authorization headers.

Completed with `internal/provider.Provider`, `OpenAICompatibleProvider`, raw HTTP non-streaming `Complete`, bearer token header injection, configured models support, placeholder streaming/health methods, OpenAI request construction/response normalization helpers, and httptest coverage for path/auth/body forwarding, response handling, configured models, and status errors that avoid leaking upstream bodies or tokens.
