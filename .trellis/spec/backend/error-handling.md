# Error Handling

> How errors are handled in this project.

---

## Overview

<!--
Document your project's error handling conventions here.

Questions to answer:
- What error types do you define?
- How are errors propagated?
- How are errors logged?
- How are errors returned to clients?
-->

(To be filled by the team)

---

## Error Types

<!-- Custom error classes/types -->

(To be filled by the team)

---

## Error Handling Patterns

<!-- Try-catch patterns, error propagation -->

(To be filled by the team)

---

## API Error Responses

<!-- Standard error response format -->

(To be filled by the team)

---

## Common Mistakes

<!-- Error handling mistakes your team has made -->

(To be filled by the team)

## Scenario: Codex Alpha Search proxy

### 1. Scope / Trigger

This contract applies to the GPT-5.6 Alpha Search routes:
`POST /v1/alpha/search` and `POST /backend-api/codex/alpha/search`.

### 2. Signatures

- `SelectCodexOAuthAuth(ctx, model, options) (*Auth, error)` selects an eligible OAuth credential.
- `NewHttpRequest` constructs the raw upstream request; `HttpRequest` executes it.
- `relayCodexAlphaSearchResponse(ctx, ginContext, response) error` copies the upstream result.

### 3. Contracts

- Read at most 16 MiB from the request body and retain the original bytes for selector metadata.
- Remove only `prompt_cache_key` and `prompt_cache_retention` from valid JSON sent upstream.
- Parse `id` and `model` independently; a type error in one advisory field must not discard the other valid field.
- Forward upstream status, `Content-Type`, and response bytes; read at most 32 MiB.
- Forward only `Version`, `User-Agent`, `Session_id`, and `X-Client-Request-Id` from client headers.
- Because Alpha Search bypasses `Execute`, explicitly call `MarkResult` once for every selected auth.
- Treat 2xx relay as success; convert transport/read errors and non-2xx bodies into `auth.Error` results.
- Do not treat a downstream response-write error as an upstream/auth failure.

### 4. Validation & Error Matrix

| Condition | HTTP result |
| --- | --- |
| Missing auth manager or no eligible auth | 503, selector error when available |
| Request body unavailable/read failure | 400 |
| Auth request construction or transport failure | 502 |
| Upstream non-2xx response | Relay upstream status and body |
| Upstream response read failure | 502 unless response already started |

### 5. Good/Base/Bad Cases

- Good: valid JSON with cache fields is forwarded without those two fields.
- Base: malformed or unaffected JSON remains byte-for-byte unchanged.
- Bad: API-key, disabled, unavailable, or actively frozen credentials are never selected.
- Home mode: accept a result only when both `AccountInfo()` is `oauth` and `Auth.Provider` is `codex`.
- Home mode: every internally skipped frozen credential advances the next dispatcher count.
- Model capability filtering applies only to `gpt-5.6*`; GPT-5.5 and earlier declarations remain unchanged.

### 6. Tests Required

- Assert sanitizer and routing helpers preserve malformed/unaffected bodies.
- Assert both routes share one handler and session/header boundaries are preserved.
- Assert OAuth selection skips API-key and active runtime-freeze credentials.
- In Home mode, assert dispatcher counts include internal freeze skips and non-Codex/API-key rejections.
- Assert 50 successful GPT-5.6 Alpha Search results trigger `limit_50`, and a 429 `usage_limit_reached` body triggers its runtime freeze.
- Integration-test the shared counter across 49 GPT-5.6 `/responses` calls plus one `/alpha/search`, then assert both routes select a backup auth after the primary freezes.
- Run focused tests, package tests, `go vet`, and `git diff --check`.

### 7. Wrong vs Correct

Wrong: translate Alpha Search through the Responses executor or copy all client headers upstream.

Correct: select OAuth through the existing scheduler, construct a raw request to the fixed Alpha Search URL, and relay the upstream result without protocol translation.
