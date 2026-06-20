---
id: api.responses-nonstream
title: Non-streaming POST /v1/responses
status: active
priority: p3
domain: api
slice: responses-api-compatibility
depends_on:
  - openai.responses-types
  - router.synthetic-models
tags:
  - api
  - responses-api
  - compatibility
context_files:
  - docs/spec.md
---

# Non-streaming POST /v1/responses

## Goal

Expose a basic non-streaming Responses API endpoint backed by the existing IR and provider routing.

## Acceptance Criteria

- [ ] Register POST /v1/responses.
- [ ] Normalize supported Responses requests to IR.
- [ ] Route requests through existing router/provider logic.
- [ ] Return a Responses-compatible non-streaming output shape for successful text responses.
- [ ] Return OpenAI-style errors for unsupported request features.
- [ ] Add integration tests with a fake provider.

## Context

Where possible, reuse Chat provider calls and translate at the edge.

## Out of Scope

- Tool calls
- Files
- Batch jobs

## Notes

Unsupported features should fail clearly rather than being silently ignored.
