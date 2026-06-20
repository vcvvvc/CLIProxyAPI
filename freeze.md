# Runtime Freeze Notes

This fork keeps runtime freeze state in:

```text
/root/.cli-proxy-api/group2/freeze.json
```

## Freeze Reasons

- `limit_50`: local policy freeze after one auth completes 50 successful requests in the current cycle.
- `usage_limit_reached`: upstream quota-limit freeze from a real provider 429 response.

Both reasons use the same freeze duration:

```text
721h
```

## Counters

`Auth.Success` is a public total success counter and must not be reset by freeze logic.

The 50-success local policy uses the internal `successFreezeCount` counter. Successful requests increment both `Auth.Success` and `successFreezeCount`.

When either `limit_50` or `usage_limit_reached` freezes an auth, only `successFreezeCount` is reset to `0`. This avoids carrying partial local-policy usage across a real upstream quota freeze.

## Expiration

A runtime freeze only blocks an auth while `NextRecoverAt` is in the future.

Expired entries in `freeze.json` are ignored by selection and scheduler logic, but they are not automatically removed from the file.
