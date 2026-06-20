---
id: ir.core-types
title: Internal normalized request and response IR
status: done
priority: p0
domain: ir
slice: first-useful-chat-completions-proxy
depends_on:
- project.bootstrap
tags:
- ir
- types
- architecture
context_files:
- docs/spec.md
---

# Internal normalized request and response IR

## Goal

Define the internal request, response, model, event, and sampling parameter types used between API and providers.

## Acceptance Criteria

- [x] Create IR structs for Request, Message, ContentPart, SamplingParams, Response, Event, and Model.
- [x] Represent text and image content parts without leaking OpenAI-specific structs into provider interfaces.
- [x] Support raw body or extra parameter preservation for forwarding unknown provider-compatible fields.
- [x] Add compile-time usage or basic tests validating JSON/raw handling as needed.

## Context

The architecture depends on normalizing once at the edge and avoiding OpenAI structs in provider interfaces.

## Out of Scope

- Responses API-specific structs
- Provider adapters

## Notes

Keep the IR minimal and avoid over-modeling features outside MVP.

Completed with provider-neutral core IR types, common role/content/modality/event constants, optional usage/error helpers, request `RawBody` and `Extra` preservation, simple modality helpers, and tests covering raw JSON preservation, text/image content, modalities, and response/event/model usability.
