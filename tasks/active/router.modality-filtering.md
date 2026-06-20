---
id: router.modality-filtering
title: Candidate filtering by modality
status: active
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

- [ ] Detect input modalities from normalized IR content parts.
- [ ] Use configured model modalities to reject text-only models for image requests.
- [ ] Continue routing text-only requests to text models.
- [ ] Return an OpenAI-style unsupported_modality error when no candidate supports the request modalities.
- [ ] Add tests for text-only, image-capable, and unsupported image routing cases.

## Context

Synthetic model fallback must not send images to text-only models.

## Out of Scope

- Model capability discovery from upstream providers

## Notes

Treat missing modality metadata conservatively or document the chosen default.
