---
id: ir.core-types
title: Internal normalized request and response IR
status: active
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

- [ ] Create IR structs for Request, Message, ContentPart, SamplingParams, Response, Event, and Model.
- [ ] Represent text and image content parts without leaking OpenAI-specific structs into provider interfaces.
- [ ] Support raw body or extra parameter preservation for forwarding unknown provider-compatible fields.
- [ ] Add compile-time usage or basic tests validating JSON/raw handling as needed.

## Context

The architecture depends on normalizing once at the edge and avoiding OpenAI structs in provider interfaces.

## Out of Scope

- Responses API-specific structs
- Provider adapters

## Notes

Keep the IR minimal and avoid over-modeling features outside MVP.
