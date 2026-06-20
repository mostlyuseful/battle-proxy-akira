---
id: router.modality-filtering
title: Candidate filtering by modality
status: done
priority: p1
domain: router
slice: multimodal-support
depends_on:
- router.synthetic-models
- openai.chat-image-parts
tags:
- router
- multimodal
- models
context_files:
- docs/spec.md
---

# Candidate filtering by modality

## Goal

Filter route candidates based on whether the request requires image-capable models.

## Acceptance Criteria

- [x] Detect input modalities from normalized IR content parts.
- [x] Use configured model modalities to reject text-only models for image requests.
- [x] Continue routing text-only requests to text models.
- [x] Return an OpenAI-style unsupported_modality error when no candidate supports the request modalities.
- [x] Add tests for text-only, image-capable, and unsupported image routing cases.

## Context

Synthetic model fallback must not send images to text-only models.

## Out of Scope

- Model capability discovery from upstream providers

## Notes

Treat missing modality metadata conservatively or document the chosen default.

Completed with router filtering based on IR `InputModalities`, direct and synthetic candidate skipping for image requests when model config lacks `image`, conservative missing-metadata-as-text-only behavior, `unsupported_modality` router errors mapped to OpenAI-style 422 API responses, and tests for text-only routing, image-capable direct routing, synthetic image candidate filtering, unsupported image requests, and missing modality metadata.
