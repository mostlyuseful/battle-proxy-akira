---
id: logging.redaction-baseline
title: Baseline redaction and no-secret guarantees
status: done
priority: p0
domain: logging
slice: metadata-logging-and-baseline-safety
depends_on:
- logging.jsonl-metadata
tags:
- logging
- security
- redaction
context_files:
- docs/spec.md
---

# Baseline redaction and no-secret guarantees

## Goal

Prevent secrets and sensitive headers from appearing in logs or error strings.

## Acceptance Criteria

- [x] Ensure Authorization headers and upstream tokens are never logged.
- [x] Review request logging and provider error paths for accidental secret inclusion.
- [x] Add redaction helpers for future structured records.
- [x] Add tests proving bearer tokens and API keys do not appear in log output for representative failures.

## Context

The security posture requires never logging Authorization or upstream tokens.

## Out of Scope

- Transcript redaction
- Image data URL hashing

## Notes

Prefer tests that search serialized logs for known sentinel secret values.

Completed with baseline redaction helpers for bearer and `sk-...` API-key patterns, JSONL record redaction before serialization, review-backed tests for redaction helpers and metadata logs, and an API failure test proving sentinel client bearer tokens/upstream API keys do not appear in response or serialized log output.
