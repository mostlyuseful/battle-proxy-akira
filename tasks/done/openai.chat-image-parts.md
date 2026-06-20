---
id: openai.chat-image-parts
title: Chat image content part parsing
status: done
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

- [x] Parse message content arrays containing text parts and image_url parts.
- [x] Accept image_url.url values that are URLs or base64 data URLs.
- [x] Preserve image detail when supplied.
- [x] Normalize images into IR ContentPart values without logging raw image data.
- [x] Add tests for text+image messages, multiple images, malformed parts, and data URLs.

## Context

Chat Completions uses image_url content parts for image input.

## Out of Scope

- Responses input_image parts
- Image fetching or validation beyond request shape

## Notes

Do not implement image generation.

Completed with multimodal Chat Completions content parsing for `text` and `image_url` parts, nested `image_url.url`/`detail` normalization into IR, data URL acceptance without fetching/validation, OpenAI-compatible request serialization for IR image parts, and tests for text+image, multiple image/data URL, malformed part rejection, and image request encoding.
