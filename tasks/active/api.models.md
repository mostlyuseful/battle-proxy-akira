---
id: api.models
title: GET /v1/models
status: active
priority: p1
domain: api
slice: first-useful-chat-completions-proxy
depends_on:
  - router.static-model
tags:
  - api
  - models
  - openai-compatible
context_files:
  - docs/spec.md
---

# GET /v1/models

## Goal

Expose configured direct and synthetic models through an OpenAI-compatible models endpoint.

## Acceptance Criteria

- [ ] Register GET /v1/models.
- [ ] Return an object=list response with model entries for exposed configured models.
- [ ] Include synthetic aliases marked expose=true once available, without breaking direct models before that task.
- [ ] Apply client auth middleware consistently.
- [ ] Add tests for model list response shape.

## Context

OpenAI-compatible clients often probe /v1/models before sending completions.

## Out of Scope

- Live upstream model discovery

## Notes

Initial model metadata can come entirely from config.
