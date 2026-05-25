---
name: grab
description: Fetch web resources through TLS fingerprint rotation and browser fallback. Use when standard HTTP clients (curl, requests, fetch) get blocked by Cloudflare or other TLS-fingerprinting CDNs — especially on academic publisher sites, CDN-protected APIs, or JS-heavy pages that require browser renderingi.
license: MIT
compatibility: Requires grab binary. Browser mode additionally requires camofox running at a reachable URL.
metadata:
  skit:
    version: 0.1.0
    keywords:
      - web-fetch
      - tls-fingerprint
      - cloudflare-bypass
      - http-client
      - browser-fallback
---

# grab

Single-binary CLI that fetches web resources through uTLS fingerprint rotation and optional browser fallback via camofox.

## When To Use

- The target site returns 403 (Cloudflare), 503, or empty responses to curl/http clients
- Fetching academic papers from biorxiv, sciencedirect, nejm, wiley, acm, plos, cell, thelancet
- Sites that require browser JS execution (stackoverflow, reddit, news.ycombinator.com)
- Any URL where a standard HTTP client fails with TLS-fingerprint-related blocking

Not for: URLs that curl already handles fine (github, wikipedia, arxiv, etc.).

## Workflow

1. **Install grab** if not already available. Check with `which grab` or by running `grab --help`. If missing, download from [releases](https://github.com/vlln/grab/releases) or build with `go install github.com/vlln/grab@latest`.

2. **Fetch with auto mode first** (default). Auto mode tries HTTP rotation first, falls back to browser only on failure:
   ```
   grab --json https://target-url.com
   ```

3. **Parse the JSON output**. On success, extract `body`, `status_code`, `engine_used`, and `headers` from the JSON envelope. The `engine_used` field tells you which engine succeeded (`chrome_120`, `chrome_131`, `safari_16_0`, or `browser`).

4. **On failure**, inspect the exit code: exit 1 means all engines exhausted (try `--mode browser` if camofox is configured, or the site may be unreachable); exit 2 means misconfiguration (check `GRAB_CAMOFOX_URL` is set when using `--mode browser`).

## Rules

- Always use `--json` when parsing output programmatically. Do not scrape stdout body text.
- `--mode auto` is the default and the right starting point for most URLs. Only set `--mode browser` when you know the target requires JS rendering.
- Set `GRAB_CAMOFOX_URL` in the environment before using `--mode browser`. Without it, browser mode fails with exit code 2.
- Use `-H "Key: Value"` for auth headers, not URL embedding.
- The `-s` (silent) flag suppresses progress output to stderr but does not affect JSON parsing.
- URLs with flags after the URL (curl-style like `grab https://example.com -o out.html`) work correctly — grab reorders arguments automatically.

## Output

JSON envelope (when `--json` is used):

```json
{
  "url": "https://example.com",
  "status_code": 200,
  "engine_used": "chrome_120",
  "fingerprint": "chrome_120",
  "from_memory": false,
  "headers": {"Content-Type": "text/html"},
  "body": "<!doctype html>..."
}
```

Exit codes: 0 = success, 1 = all engines exhausted, 2 = misconfiguration.

## Browser Fallback

Browser mode delegates to a camofox instance for full Chromium rendering:

```
GRAB_CAMOFOX_URL=http://localhost:9377 grab --json --mode browser https://js-heavy-site.com
```

Set `GRAB_CAMOFOX_URL` as an environment variable. In `--mode auto`, grab only falls back to camofox after all HTTP fingerprints have been tried — no browser is needed for most sites.
