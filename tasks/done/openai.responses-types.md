---
id: openai.responses-types
title: Responses API request and response structs
status: done
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

- [x] Define request structs for model, input, stream, and common sampling parameters.
- [x] Parse text input forms needed by expected clients.
- [x] Parse input_image parts and normalize them into IR image content parts.
- [x] Define response structs sufficient for MVP non-streaming output.
- [x] Add tests for text input, image input, and sampling parameter mapping.

## Context

Responses API is recommended but lower priority than Chat Completions for MVP compatibility.

## Out of Scope

- Full Responses feature parity
- Tools
- Files

## Notes

Reuse the core IR rather than duplicating routing logic.

Implemented in `internal/openai/responses_types.go`: `ResponseRequest` (model, input, instructions, stream, temperature, top_p, max_output_tokens; unknown top-level fields preserved in `Extra` and raw body in `RawBody`), `ResponseInput` (plain string or array of `ResponseInputItem`), `ResponseInputItem` (EasyInputMessage with role + string or multimodal content), `ResponseInputContentPart` (`input_text`/`input_image` with image_url/file_id/detail), and `Response`/`ResponseOutputItem`/`ResponseOutputContent`/`ResponseUsage`/`ResponseError` for non-streaming output. `ToIR` normalizes text and image inputs (image parts become IR `input_image` content parts; instructions become a developer message; `max_output_tokens` maps to `SamplingParams.MaxCompletionTokens`), and `ResponseFromIR`+`Response.ToIR` round-trip assistant output text and token usage with finish-reason/status reconciliation (`length`<->`incomplete`, otherwise `completed`/`stop`). Unknown input/output item and content types are preserved verbatim in `Raw`/`Extra`. Tests cover text input, string-content message items, multimodal image input, unknown-field preservation and marshal round-trip, missing-model/empty-input/unsupported-item-type/image-without-source rejection, and response round-trip including length/incomplete mapping and error cases for output without a message or with an unsupported output content type.
