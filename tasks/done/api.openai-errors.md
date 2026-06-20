---
id: api.openai-errors
title: OpenAI-compatible error responses
status: done
priority: p0
domain: api
slice: bootable-proxy-foundation
depends_on:
- project.bootstrap
tags:
- api
- errors
- compatibility
context_files:
- docs/spec.md
---

# OpenAI-compatible error responses

## Goal

Centralize OpenAI-style error JSON generation and HTTP status mapping.

## Acceptance Criteria

- [x] Implement a reusable OpenAI-compatible error response shape.
- [x] Map internal error codes such as invalid_request, unknown_model, no_available_model, unsupported_modality, and upstream_error to suitable HTTP statuses.
- [x] Ensure handlers can write consistent error JSON with content-type application/json.
- [x] Add handler/unit tests for representative error responses.

## Context

The spec defines the target error object with message, type, param, and code fields.

## Out of Scope

- Provider-specific error parsing
- Streaming mid-flight error events

## Notes

Prefer a small internal error type over ad hoc handler responses.

Completed with `ProxyError`, OpenAI error envelope/shape, status/type mapping helpers, `WriteOpenAIError`, and unit tests for representative mappings and response JSON.
