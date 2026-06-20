---
id: router.synthetic-models
title: Synthetic model alias expansion
status: done
priority: p0
domain: router
slice: synthetic-model-aliases-and-routing-value
depends_on:
- router.static-model
tags:
- router
- synthetic-models
- models
context_files:
- docs/spec.md
---

# Synthetic model alias expansion

## Goal

Resolve configured synthetic model names such as coding into ordered provider/model candidates.

## Acceptance Criteria

- [x] Load synthetic model definitions from config.
- [x] Resolve a requested alias to its ordered candidate list.
- [x] Support first_available strategy for MVP.
- [x] Expose aliases marked expose=true through /v1/models.
- [x] Return responses using the requested model name by default while preserving actual provider/model for logs.
- [x] Add tests for alias expansion, unknown aliases, and model response rewriting.

## Context

Synthetic models are routing policies, not fake providers.

## Out of Scope

- Least-cost strategy
- Per-client policy

## Notes

Candidate strings should reference provider/model pairs that exist in config.

Completed with synthetic alias expansion in `StaticRouter`, ordered `first_available` candidates, exposed synthetic aliases via `Models`, route metadata that preserves requested alias and actual provider/model, request/response model rewrite helpers, and tests for alias order, missing provider instances, no available providers, exposed/hidden aliases, unknown aliases, and response rewriting.
