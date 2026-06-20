---
id: logging.image-hash
title: Safe image logging metadata
status: active
priority: p2
domain: logging
slice: multimodal-support
depends_on:
  - logging.redaction-baseline
  - openai.chat-image-parts
tags:
  - logging
  - images
  - redaction
context_files:
  - docs/spec.md
---

# Safe image logging metadata

## Goal

Log safe metadata for image inputs without storing raw base64 data URLs.

## Acceptance Criteria

- [ ] Detect base64 data URL image inputs in normalized requests.
- [ ] Record hash, byte length, and MIME type for image data URLs when logging is enabled.
- [ ] Avoid logging raw base64 image data by default.
- [ ] Preserve external image URLs only according to the selected redaction policy.
- [ ] Add tests verifying raw image data is absent and hash metadata is present.

## Context

The spec requires image data URLs to be hash-only by default in logs.

## Out of Scope

- Full transcript logging controls

## Notes

Use SHA-256 from the stdlib crypto package.
