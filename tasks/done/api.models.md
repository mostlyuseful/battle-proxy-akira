---
id: api.models
title: GET /v1/models
status: done
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

- [x] Register GET /v1/models.
- [x] Return an object=list response with model entries for exposed configured models.
- [x] Include synthetic aliases marked expose=true once available, without breaking direct models before that task.
- [x] Apply client auth middleware consistently.
- [x] Add tests for model list response shape.

## Context

OpenAI-compatible clients often probe /v1/models before sending completions.

## Out of Scope

- Live upstream model discovery

## Notes

Initial model metadata can come entirely from config.

Completed with `/v1/models` registration in `NewServer`, a `ModelLister` API hook compatible with the router's configured direct/synthetic model listing, OpenAI-compatible `{object:"list", data:[...]}` model response shape, optional client-auth middleware integration, and tests for model list shape, middleware application, lister failures, and default empty model listing.
