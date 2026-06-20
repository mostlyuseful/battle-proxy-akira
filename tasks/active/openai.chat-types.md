---
id: openai.chat-types
title: Chat Completions request and response structs
status: active
priority: p0
domain: openai
slice: first-useful-chat-completions-proxy
depends_on:
  - ir.core-types
tags:
  - openai
  - chat-completions
  - json
context_files:
  - docs/spec.md
---

# Chat Completions request and response structs

## Goal

Parse OpenAI-compatible Chat Completions JSON and translate basic text requests to the internal IR.

## Acceptance Criteria

- [ ] Define Chat Completions request structs for model, messages, stream, and common sampling parameters.
- [ ] Accept simple string message content and convert it to text content parts in IR.
- [ ] Capture temperature, top_p, max_tokens, max_completion_tokens, stop, presence_penalty, frequency_penalty, and seed.
- [ ] Preserve unknown or raw request fields where practical for compatible upstream forwarding.
- [ ] Add tests for text-only request normalization and sampling parameter mapping.

## Context

Prioritize POST /v1/chat/completions for strict OpenAI compatibility.

## Out of Scope

- Image content parts
- Responses API

## Notes

Do not introduce the OpenAI SDK.
