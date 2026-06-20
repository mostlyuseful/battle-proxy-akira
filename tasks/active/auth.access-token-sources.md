---
id: auth.access-token-sources
title: Env, file, and command access-token sources
status: active
priority: p2
domain: auth
slice: access-token-auth-and-exhaustion-buffering
depends_on:
  - auth.bearer-env-source
tags:
  - auth
  - access-token
  - codex
context_files:
  - docs/spec.md
---

# Env, file, and command access-token sources

## Goal

Add access-token source implementations suitable for subscription-style provider auth.

## Acceptance Criteria

- [ ] Implement env_access_token reading a configured environment variable.
- [ ] Implement file_access_token reading a configured local file.
- [ ] Implement command_access_token invoking a configured command and parsing JSON output.
- [ ] Support command output containing access_token and optional expires_at.
- [ ] Reject missing, malformed, or empty tokens without leaking token values.
- [ ] Add unit tests for each source using temporary env/files/fake commands.

## Context

Treat Codex subscription-style auth as an access-token source, not a hardcoded OAuth hack.

## Out of Scope

- Device flow
- Scraping local Codex auth files

## Notes

Do not depend on undocumented ~/.codex/auth.json behavior.
