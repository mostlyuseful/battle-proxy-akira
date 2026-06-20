---
id: openai.chat-image-parts
title: Chat image content part parsing
status: active
priority: p1
domain: openai
slice: multimodal-support
depends_on:
  - openai.chat-types
tags:
  - openai
  - multimodal
  - images
context_files:
  - docs/spec.md
---

# Chat image content part parsing

## Goal

Accept OpenAI Chat Completions multimodal image content parts and normalize them into IR.

## Acceptance Criteria

- [ ] Parse message content arrays containing text parts and image_url parts.
- [ ] Accept image_url.url values that are URLs or base64 data URLs.
- [ ] Preserve image detail when supplied.
- [ ] Normalize images into IR ContentPart values without logging raw image data.
- [ ] Add tests for text+image messages, multiple images, malformed parts, and data URLs.

## Context

Chat Completions uses image_url content parts for image input.

## Out of Scope

- Responses input_image parts
- Image fetching or validation beyond request shape

## Notes

Do not implement image generation.
