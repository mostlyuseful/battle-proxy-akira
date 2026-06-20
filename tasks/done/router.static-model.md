---
id: router.static-model
title: Direct provider/model resolution
status: done
priority: p0
domain: router
slice: first-useful-chat-completions-proxy
depends_on:
- config.json-loader
- provider.openai-nonstream
tags:
- router
- models
- providers
context_files:
- docs/spec.md
---

# Direct provider/model resolution

## Goal

Resolve direct provider/model requests to configured provider route candidates.

## Acceptance Criteria

- [x] Define route candidate metadata containing provider name and provider model.
- [x] Resolve direct model names where configured for a provider.
- [x] Support provider:model notation if adopted by the implementation.
- [x] Return unknown_model or no_available_model errors for unresolved requests.
- [x] Add tests for valid direct model and unknown model cases.

## Context

This is the routing base that synthetic aliases and fallback will extend later.

## Out of Scope

- Synthetic aliases
- Modality filtering
- Circuit-breaker state

## Notes

Keep routing policy deterministic.

Completed with `internal/router.StaticRouter`, route candidate metadata, full router interface with no-op success/failure hooks, direct model and `provider:model` resolution, deterministic lexicographic provider selection, `unknown_model`/`no_available_model` routing errors, and tests for direct resolution, provider notation, deterministic duplicate handling, unknown model/provider, missing provider instance, and canceled context.
