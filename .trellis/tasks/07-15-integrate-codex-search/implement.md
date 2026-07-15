# Codex Search implementation plan

## Execution rules

- Work on `cpa-dev`; do not merge or rebase `main`.
- Preserve pre-existing `AGENTS.md`, `.freeze.md.swp`, and `.trellis/` changes.
- Each turn implements one single-responsibility function or one independently
  compilable wiring change; logic diff stays below 35 lines where applicable.
- After every slice run its focused checks and stop for explicit continuation.
- Do not modify the GPT-5.5 Responses Search path.

## Slices

### 1. Body sanitizer — completed

- `internal/api/server.go`: pure sanitizer for the two cache fields.
- Verification: `go test ./internal/api`, `go vet ./internal/api`,
  `git diff --check`.

### 2. Replace interim selector bridge

- Remove the unused generic `SelectAuth` added during exploration.
- Add the narrow `SelectCodexOAuthAuth` adapter consumed by Alpha Search,
  classifying auth through existing `AccountInfo()` data.
- Reuse `pickNext`, `tried`, HOME count metadata, and existing freeze refresh;
  do not alter `MarkResult` or scheduler policy.
- Add focused selector tests only when this slice can prove OAuth filtering,
  unavailable auth errors, and freeze exclusion without widening the slice.
- Verify: `go test ./sdk/cliproxy/auth`, `go vet ./sdk/cliproxy/auth`.

### 3. Alpha Search routing parser

- Add a pure helper that extracts trimmed `id` and `model` from the bounded
  request body without making routing or auth decisions.
- Keep malformed JSON safe and return empty routing values.
- Verify `internal/api` still compiles; route registration waits until the
  handler boundary exists.

### 4. Request preparation

- Add the smallest helpers needed to read the bounded body, extract `id` and
  `model`, clone headers, and apply sanitation.
- Preserve the original body for selector metadata and use the sanitized body
  only for the upstream request/logging payload.
- Verify malformed, empty, and oversized body behavior.

### 5. Direct upstream request and relay

- Add the handler/forwarding helpers in separate turns if needed to keep each
  function small.
- Construct the request through `NewHttpRequest`, execute through
  `HttpRequest`, and relay status/content type/body.
- Reuse existing request/response logging helpers and close response bodies.
- Verify upstream success, non-2xx relay, read failure, and auth transport
  failure.

### 6. Route registration wiring

- Add both Alpha Search POST routes to the existing authenticated groups after
  the handler boundary compiles.
- No legacy Responses route behavior change.
- Verify route registration with focused `internal/api` tests.

### 7. Alpha Search integration tests

- Test both route aliases share the handler.
- Test OAuth selection, API-key rejection, disabled/unavailable auth, active
  freeze exclusion, session header propagation, body sanitation, and response
  relay.
- Confirm legacy Responses Search tests remain unchanged and passing.

### 8. Model capability filter

- Add a pure fail-closed helper for template/provider eligibility.
- Wire it into the current model response builder without importing the full
  remote catalog refresh.
- Preserve GPT-5.6 model entries, reasoning levels, and existing GPT-5.5
  metadata.
- Verify: `go test ./sdk/api/handlers/openai`.

### 9. Full affected-scope verification

- `go test ./internal/api`
- `go test ./sdk/cliproxy/auth`
- `go test ./sdk/api/handlers/openai`
- `go test ./sdk/auth`
- `go vet ./internal/api ./sdk/cliproxy/auth ./sdk/api/handlers/openai ./sdk/auth`
- `git diff --check`
- Review the final diff for accidental changes to `freeze.md`, freeze store,
  counters, GPT-5.5 routes, or unrelated upstream files.

## Quality gates

- No slice advances while its focused checks fail.
- Any failure involving `limit_50`, `usage_limit_reached`, 721-hour duration,
  `Auth.Success`, `successFreezeCount`, or freeze sidecar persistence blocks the
  next slice.
- No commit or task archive until the complete acceptance matrix passes.
