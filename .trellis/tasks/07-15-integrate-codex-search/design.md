# Codex Search integration design

## First principles

The problem is not “merge Search commits”; it is “let GPT-5.6 send a second
Search request shape without breaking the GPT-5.5 path or private auth policy.”
Therefore the smallest safe design adds one raw proxy path and connects it to
the existing selector.

## Compatibility boundaries

### Legacy path: GPT-5.5 and earlier

```text
client → /v1/responses or /backend-api/codex/responses
       → existing handler/executor/translator
       → existing auth selector
       → upstream Responses API
```

This path is not modified.

### New path: GPT-5.6 Alpha Search

```text
client → /v1/alpha/search or /backend-api/codex/alpha/search
       → parse id/model and retain raw body
       → existing selector with required kind=OAuth
       → remove two upstream-incompatible cache fields
       → direct HTTP request to Alpha Search upstream
       → log and relay upstream result
```

The two paths share auth selection and runtime state, but not request
translation or execution.

## Auth-selection contract

The route needs a selection-only manager method because it must construct a
raw HTTP request after choosing credentials; calling normal `Execute` would
invoke the model executor and translator. The final upstream consumer is a
kind-aware method, not a generic Search scheduler.

The adapter is named `SelectCodexOAuthAuth` in this fork. It uses the existing
`AccountInfo()` classification instead of importing the unrelated upstream
`AuthKind` type. It must:

1. Pass provider `codex`, requested model, original body, and cloned headers to
   the existing `pickNext` path.
2. Skip selected auths whose account type is not `oauth`, using the existing
   `tried` mechanism and HOME selection count behavior.
3. Preserve disabled/unavailable/model/session filtering already implemented by
   the selector.
4. Preserve active runtime freezes. If the selected auth needs a sidecar
   refresh, use the existing runtime-freeze refresh path; do not duplicate
   freeze rules in `server.go`.
5. Return the selector's existing typed errors without mutating result counters.

The interim generic `SelectAuth` added during exploration is not part of the
final design and should be removed before the route is wired.

## HTTP contract

- Request body: read at most 16 MiB.
- Routing fields: `id` and `model` are advisory JSON fields; malformed routing
  JSON must not panic or prevent a clear selection error.
- Session affinity: copy the incoming headers and set `X-Session-ID` from a
  non-empty request `id`.
- Upstream headers: set `Content-Type`, `Accept`, and `Originator`; forward
  `Version`, `User-Agent`, `Session_id`, and `X-Client-Request-Id` when present;
  add `Chatgpt-Account-Id` from selected auth metadata.
- Body sanitation: delete only `prompt_cache_key` and
  `prompt_cache_retention`; keep the original bytes on parse/marshal failure.
- Upstream URL: fixed Alpha Search endpoint; no model-dependent URL rewrite.
- Response body: read at most 32 MiB, preserve upstream status and content
  type, and return the bytes unchanged.

## Error contract

- Missing auth manager or no eligible auth: service-unavailable style error
  using the selector's status when available.
- Request body read or malformed forwarding input: bad request.
- Auth request construction or upstream transport failure: bad gateway.
- Upstream non-2xx response: relay its status and body rather than converting
  it to a local success.
- Record every selected auth result through `MarkResult`: 2xx relay is success;
  transport/read failures and non-2xx responses are failures using the upstream
  status/body so `limit_50` and `usage_limit_reached` freezes remain active.
- Response close/read errors: log through existing conventions; do not leak
  credentials or raw secrets in new logs.

## Model capability contract

The current model builder must retain existing GPT-5.6 metadata while applying
the upstream rule only to GPT-5.6 entries: `supports_search_tool` is true only
for a real GPT-5.6 template whose provider set is non-empty and contains only
`codex`. Unknown, non-template, mixed-provider, or unavailable-provider
GPT-5.6 cases fail closed. GPT-5.5 and earlier model declarations remain
unchanged because they use the existing Responses Search path. The complete
remote catalog refresh is not part of this port.

## Observability and security

Reuse existing request/response logging helpers. Confirm their redaction
behavior before passing headers, auth metadata, and sanitized body. Do not add
new secret logging. Keep the existing auth account ID behavior only where the
helper already supports it.

## Rollback

- Route-only rollback: remove the two route registrations and Alpha handler;
  legacy Responses Search remains intact.
- Selector rollback: remove only the kind-aware adapter and its tests; do not
  revert private freeze code.
- Model rollback: disable only the new capability filter while retaining
  existing model metadata.
- Any freeze regression blocks further slices and restores the last verified
  selector implementation before continuing.
