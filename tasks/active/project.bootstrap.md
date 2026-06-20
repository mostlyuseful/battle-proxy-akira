---
id: project.bootstrap
title: Go module and server skeleton
status: active
priority: p0
domain: project
slice: bootable-proxy-foundation
depends_on:
  []
tags:
  - go
  - server
  - foundation
context_files:
  - docs/spec.md
---

# Go module and server skeleton

## Goal

Create the minimal Go project structure and a bootable HTTP server with health endpoints.

## Acceptance Criteria

- [ ] Initialize a Go module for the proxy.
- [ ] Create the planned package skeleton under cmd/llm-proxy and internal/* where immediately needed.
- [ ] Implement GET /healthz returning a successful health response.
- [ ] Implement GET /readyz returning a successful readiness response.
- [ ] Add basic tests or build checks showing the server compiles and endpoints are wired.

## Context

Use stdlib net/http and avoid framework dependencies. Keep the skeleton small and easy to extend.

## Out of Scope

- Provider adapters
- Chat completions
- Config loading beyond minimal defaults

## Notes

This is the first task and should leave the repo buildable.
