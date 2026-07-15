# Integrate Codex Search updates

## Goal

Add the standalone Codex Alpha Search path required by GPT-5.6 while keeping
the existing Responses-embedded Search path used by GPT-5.5 unchanged. Reuse
the current auth selector and preserve the fork's intentional runtime-freeze
policy.

## Confirmed facts

- `cpa-dev` already supports Responses-embedded `web_search` through
  `/v1/responses` and `/backend-api/codex/responses`.
- The current branch does not register `/v1/alpha/search` or
  `/backend-api/codex/alpha/search`.
- The current model catalog already contains GPT-5.6 entries with
  `supports_search_tool` metadata.
- Upstream final behavior is represented by `46e2894a`, `9c3f7207`,
  `55f4d6ed`, and `411d7d41`; model capability hardening is represented by
  `ceaeb75d` and `e73aad2e`.
- The upstream final handler calls `SelectAuthByKind`; the earlier generic
  `SelectAuth` is not used by the final route. Current `cpa-dev` does not yet
  have the upstream `AuthKind` type, but `Auth.AccountInfo()` already
  distinguishes `oauth` from `api_key`.
- The fork freezes an auth after 50 successful requests for 721 hours,
  persists state in `/root/.cli-proxy-api/group2/freeze.json`, and must keep
  `Auth.Success`, `successFreezeCount`, `limit_50`, and
  `usage_limit_reached` semantics unchanged.
- A full `main` merge would overlap private changes in `conductor.go`, model
  handling, and runtime-freeze storage; a selective behavioral port is safer.

## Requirements

R1. Keep GPT-5.5 and earlier Responses Search behavior unchanged.

R2. Register both GPT-5.6 Alpha Search routes:

- `POST /v1/alpha/search`
- `POST /backend-api/codex/alpha/search`

R3. Forward the Alpha Search payload to
`https://chatgpt.com/backend-api/codex/alpha/search` without protocol
translation.

R4. Reuse the existing selector for provider `codex`, requested model, session
headers, and auth state; add only the narrow auth-kind adapter required by the
route. Do not create a second Search selection algorithm.

R5. Select OAuth credentials only. API-key credentials and unavailable,
disabled, or actively frozen credentials must not be used.

R6. Preserve the upstream request contract: body limit, routing fields,
session-affinity header, required upstream headers, account ID, status code,
content type, and response body.

R7. Remove only `prompt_cache_key` and `prompt_cache_retention` before
forwarding; malformed JSON or serialization failure must leave the original
body intact.

R8. Keep model capability output fail-closed for non-template, unknown, or
mixed-provider models without importing the full remote catalog refresh.

R9. Preserve the runtime-freeze counters, duration, reasons, persistence path,
expiration behavior, and existing Responses execution flow.

## Acceptance criteria

- AC1: GPT-5.5 Responses Search tests and request/response behavior remain
  unchanged.
- AC2: Both Alpha Search routes reach the same handler and proxy a valid
  request to the upstream Alpha Search URL.
- AC3: Selection uses the existing scheduler/session-affinity path, rejects
  non-OAuth auths, and skips active runtime freezes.
- AC4: The request sanitizer removes exactly the two incompatible fields and
  preserves invalid or unaffected bodies.
- AC5: Upstream status, content type, and response body are returned to the
  client; selection, request, upstream, and body-read errors map to explicit
  HTTP errors.
- AC6: GPT-5.6 models advertise Search only when the current provider/template
  contract permits it; existing GPT-5.6 metadata and reasoning settings remain.
- AC7: `limit_50` and `usage_limit_reached` freeze behavior remains unchanged,
  including 721-hour duration and sidecar persistence.
- AC8: Focused tests, package tests, `go vet`, and `git diff --check` pass.

## Out of scope

- Merging all 416 commits from `main`.
- Replacing `conductor.go` with the upstream version.
- Changing GPT-5.5 Responses Search, translator semantics, or normal executor
  routing.
- Importing unrelated xAI Search, translator, UI, release, or provider work.
- Introducing the complete remote model-catalog refresh subsystem unless a
  focused test proves the current provider lookup is insufficient.

## Decisions

- Port final behavior, not intermediate commit history.
- Remove the interim generic `SelectAuth` bridge; implement the narrow
  `SelectCodexOAuthAuth` adapter consumed by Alpha Search using the existing
  `AccountInfo()` classification.
- Treat the existing Responses Search path as a regression boundary.
