---
id: router.synthetic-models
title: Synthetic model alias expansion
status: active
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

- [ ] Load synthetic model definitions from config.
- [ ] Resolve a requested alias to its ordered candidate list.
- [ ] Support first_available strategy for MVP.
- [ ] Expose aliases marked expose=true through /v1/models.
- [ ] Return responses using the requested model name by default while preserving actual provider/model for logs.
- [ ] Add tests for alias expansion, unknown aliases, and model response rewriting.

## Context

Synthetic models are routing policies, not fake providers.

## Out of Scope

- Least-cost strategy
- Per-client policy

## Notes

Candidate strings should reference provider/model pairs that exist in config.
