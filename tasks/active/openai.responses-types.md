---
id: openai.responses-types
title: Responses API request and response structs
status: active
priority: p3
domain: openai
slice: responses-api-compatibility
depends_on:
  - ir.core-types
tags:
  - openai
  - responses-api
  - json
context_files:
  - docs/spec.md
---

# Responses API request and response structs

## Goal

Represent enough of the OpenAI Responses API shape to normalize text and image requests into IR.

## Acceptance Criteria

- [ ] Define request structs for model, input, stream, and common sampling parameters.
- [ ] Parse text input forms needed by expected clients.
- [ ] Parse input_image parts and normalize them into IR image content parts.
- [ ] Define response structs sufficient for MVP non-streaming output.
- [ ] Add tests for text input, image input, and sampling parameter mapping.

## Context

Responses API is recommended but lower priority than Chat Completions for MVP compatibility.

## Out of Scope

- Full Responses feature parity
- Tools
- Files

## Notes

Reuse the core IR rather than duplicating routing logic.
