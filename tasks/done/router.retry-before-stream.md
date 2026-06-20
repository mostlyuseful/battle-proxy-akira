---
id: router.retry-before-stream
title: Basic retry before first streamed token
status: done
priority: p1
domain: router
slice: streaming-mvp
depends_on:
- api.chat-stream
- router.static-model
tags:
- router
- retry
- streaming
context_files:
- docs/spec.md
---

# Basic retry before first streamed token

## Goal

Allow safe retry or fallback only before any streamed data has been sent to the client.

## Acceptance Criteria

- [x] Track whether a streaming response has emitted the first event to the client.
- [x] Permit retry/fallback for retryable failures before the first event.
- [x] Prevent fallback after the first event is emitted.
- [x] Return or emit an appropriate stream_interrupted behavior for mid-stream upstream failure.
- [x] Add tests for pre-stream retry and post-token no-fallback behavior.

## Context

The spec explicitly locks the design choice: never fallback after first streamed token.

## Out of Scope

- Full candidate fallback policy
- Circuit breaker state

## Notes

If only one route exists, this still establishes the safety boundary.

Completed with streaming candidate retry orchestration in the Chat Completions handler: retryable `Provider.Stream` failures and retryable pre-event stream errors can advance to the next candidate before response headers/data are sent; after the first SSE event is forwarded, fallback is disabled and a `stream_interrupted` SSE error is emitted. Logging records the final selected candidate and retry count. Tests cover pre-stream retry/fallback and post-token no-fallback behavior.
