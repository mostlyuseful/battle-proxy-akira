---
id: logging.image-hash
title: Safe image logging metadata
status: done
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

- [x] Detect base64 data URL image inputs in normalized requests.
- [x] Record hash, byte length, and MIME type for image data URLs when logging is enabled.
- [x] Avoid logging raw base64 image data by default.
- [x] Preserve external image URLs only according to the selected redaction policy.
- [x] Add tests verifying raw image data is absent and hash metadata is present.

## Context

The spec requires image data URLs to be hash-only by default in logs.

## Out of Scope

- Full transcript logging controls

## Notes

Use SHA-256 from the stdlib crypto package.

Completed with safe `image_inputs` metadata on request log records, extracted from normalized IR in Chat Completions handlers. Data URLs log source, MIME type, decoded byte length, and SHA-256 of decoded bytes; raw base64/data URLs are not serialized. External URLs are redacted by default with only `url_redacted=true`. Tests cover metadata extraction, JSONL raw-data absence/hash presence, and API log propagation.
