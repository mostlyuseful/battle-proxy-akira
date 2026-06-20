---
id: openai.chat-types
title: Chat Completions request and response structs
status: done
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

- [x] Define Chat Completions request structs for model, messages, stream, and common sampling parameters.
- [x] Accept simple string message content and convert it to text content parts in IR.
- [x] Capture temperature, top_p, max_tokens, max_completion_tokens, stop, presence_penalty, frequency_penalty, and seed.
- [x] Preserve unknown or raw request fields where practical for compatible upstream forwarding.
- [x] Add tests for text-only request normalization and sampling parameter mapping.

## Context

Prioritize POST /v1/chat/completions for strict OpenAI compatibility.

## Out of Scope

- Image content parts
- Responses API

## Notes

Do not introduce the OpenAI SDK.

Completed with stdlib-only Chat Completions request/response structs, parser preserving raw body and unknown fields, text-only normalization to IR, sampling/stop mapping, basic IR-to-Chat response conversion, and tests for text normalization, sampling, unknown preservation, unsupported multimodal arrays, and response JSON.
