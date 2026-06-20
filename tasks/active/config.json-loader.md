---
id: config.json-loader
title: JSON config loading and validation
status: active
priority: p0
domain: config
slice: bootable-proxy-foundation
depends_on:
  - project.bootstrap
tags:
  - config
  - json
  - validation
context_files:
  - docs/spec.md
---

# JSON config loading and validation

## Goal

Load and validate stdlib-only JSON configuration for server, auth, providers, synthetic models, and logging.

## Acceptance Criteria

- [ ] Define config structs matching the MVP config shape from the spec.
- [ ] Load config from an explicit path and support sensible local defaults when omitted.
- [ ] Validate required provider fields, model metadata, server settings, and auth settings.
- [ ] Return actionable validation errors without leaking secrets.
- [ ] Add unit tests for valid config, missing required fields, and invalid provider/model references.

## Context

Keep JSON as the initial config format to preserve zero required third-party runtime dependencies.

## Out of Scope

- Runtime config reload
- TOML/YAML support

## Notes

This should provide the typed configuration other tasks consume.
