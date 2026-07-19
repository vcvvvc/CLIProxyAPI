# Runtime Freeze Notes

This fork keeps runtime freeze state in:

```text
/root/.cli-proxy-api/group2/freeze.json
```

## Freeze Reasons

- `limit_<N>`: local policy freeze after one auth reaches the effective successful-request limit `N` in the current cycle.
- `usage_limit_reached`: upstream quota-limit freeze from a real provider 429 response.

`limit_50` remains valid for the default limit and for existing persisted records. New records include the effective limit, such as `limit_120`.

All runtime freeze reasons use the same freeze duration:

```text
721h
```

## Counters

`Auth.Success` is a public total success counter and must not be reset by freeze logic.

The local policy uses the internal `successFreezeCount` counter. Successful requests increment both `Auth.Success` and `successFreezeCount`.

`success-freeze-limit` configures the cycle limit. Missing or non-positive values use the default of `50`.

When either `limit_<N>` or `usage_limit_reached` freezes an auth, only `successFreezeCount` is reset to `0`. This avoids carrying partial local-policy usage across a real upstream quota freeze.

## Expiration

A runtime freeze only blocks an auth while `NextRecoverAt` is in the future.

Expired entries in `freeze.json` are ignored by selection and scheduler logic, but they are not automatically removed from the file.
