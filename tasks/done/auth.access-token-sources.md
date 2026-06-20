---
id: auth.access-token-sources
title: Env, file, and command access-token sources
status: done
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

- [x] Implement env_access_token reading a configured environment variable.
- [x] Implement file_access_token reading a configured local file.
- [x] Implement command_access_token invoking a configured command and parsing JSON output.
- [x] Support command output containing access_token and optional expires_at.
- [x] Reject missing, malformed, or empty tokens without leaking token values.
- [x] Add unit tests for each source using temporary env/files/fake commands.

## Context

Treat Codex subscription-style auth as an access-token source, not a hardcoded OAuth hack.

## Out of Scope

- Device flow
- Scraping local Codex auth files

## Notes

Do not depend on undocumented ~/.codex/auth.json behavior.

Completed with env/file/command access-token `TokenSource` implementations wired through `NewTokenSource`. Command sources parse JSON `access_token` and optional RFC3339 `expires_at`, reject malformed/empty/expired outputs without including token values or command output in errors, and tests cover env, temporary files, fake shell commands, expiry handling, missing/malformed values, unsupported auth, and context cancellation.
