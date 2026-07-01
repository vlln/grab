---
name: grab
description: Use this skill when standard HTTP clients (curl, requests, fetch) get blocked by Cloudflare or other TLS-fingerprinting CDNs — especially on academic publisher sites, CDN-protected APIs, or JS-heavy pages that require browser rendering.
license: MIT
metadata:
  author: vlln
  version: "0.1.0"
requires:
  bins:
    - grab
---

# grab

Fetch web resources from sites that block standard HTTP clients, using TLS fingerprint rotation and browser rendering fallback.

## Trigger Keywords

web scraping, fetch, download, curl, http, cloudflare, 403, blocked, bypass, TLS fingerprint, academic publisher, JS rendering, browser rendering, CDN bypass

## Capabilities

- Bypass Cloudflare and other TLS-fingerprinting CDN blocks that return 403, 503, or empty responses
- Fetch from academic publisher sites: biorxiv, sciencedirect, nejm, wiley, acm, plos, cell, thelancet
- Render JavaScript-heavy pages that require a full browser engine (stackoverflow, reddit, news.ycombinator.com)
- Produce structured JSON output for programmatic parsing

Do not use for URLs that curl handles fine (github, wikipedia, arxiv, etc.).

## Workflow

1. **Fetch with auto mode first** (default). Auto mode tries HTTP fingerprint rotation first, falls back to browser rendering only on failure:
   ```
   grab --json https://target-url.com
   ```

2. **Parse the JSON output**. On success, extract `body`, `status_code`, `engine_used`, and `headers` from the JSON envelope. The `engine_used` field tells you which engine succeeded (`chrome_120`, `chrome_131`, `safari_16_0`, or `browser`).

3. **On failure**, inspect the exit code: exit 1 means all engines exhausted (try `--mode browser` if browser rendering is configured, or the site may be unreachable); exit 2 means misconfiguration (set `GRAB_CAMOFOX_URL` when using `--mode browser`).

## Rules

- Always use `--json` when parsing output programmatically. Do not scrape stdout body text.
- `--mode auto` is the default and the right starting point for most URLs. Only set `--mode browser` when you know the target requires JS rendering.
- Set `GRAB_CAMOFOX_URL` in the environment before using `--mode browser`. Without it, browser mode fails with exit code 2.
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

## Browser Rendering

Browser mode delegates to a browser rendering service for full Chromium execution:

```
GRAB_CAMOFOX_URL=http://localhost:9377 grab --json --mode browser https://js-heavy-site.com
```

In `--mode auto`, grab only falls back to browser rendering after all HTTP fingerprints have been tried — no browser is needed for most sites.

## Gotchas

- **Memory cache staleness.** grab caches successful fingerprints per domain in `~/.grab/memory.json`. If a previously-working fingerprint starts failing (e.g. the site updated its CDN config), use `--no-memory` to force a fresh rotation.
- **Browser-only sites are slower in auto mode.** `--mode auto` tries all HTTP fingerprints before falling back to browser. If you already know the site requires JS rendering, use `--mode browser` directly to skip the HTTP attempts.
- **200 does not mean content.** Some Cloudflare-protected sites return HTTP 200 with a JS challenge page instead of the real content. Inspect the body for Cloudflare challenge indicators (e.g. `cf-challenge`, `_cf_chl_opt`) and retry with `--mode browser` if detected.
- **JSON body is always a string.** The `body` field in JSON output is always a UTF-8 string. Binary content or non-UTF-8 encodings will be mangled — use `-o <file>` instead of `--json` for binary downloads.
- **Large bodies in JSON.** The `--json` output embeds the entire body in a single JSON field. For large pages, prefer `-o <file>` to avoid memory pressure.