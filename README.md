# grab

Single-binary CLI that fetches web resources through TLS fingerprint rotation and optional browser fallback. Designed for agents — one URL in, one response out, zero runtime dependencies.

## Skills

This repository also ships as a [Claude Code skill](https://docs.anthropic.com/en/docs/claude-code/skills). Install it with [skit](https://github.com/vlln/skit):

```
skit install github:vlln/grab --skill grab
```

| Skill | Description |
|-------|-------------|
| `grab` | Fetch web resources through TLS fingerprint rotation and browser fallback — bypass Cloudflare blocking on academic sites, CDN-protected APIs, and JS-heavy pages |


## Install

### Download binary (recommended)

Grab the latest binary from [releases](https://github.com/vlln/grab/releases):

```bash
# Linux x86_64
curl -L -o grab https://github.com/vlln/grab/releases/latest/download/grab-linux-amd64
chmod +x grab

# macOS Intel
curl -L -o grab https://github.com/vlln/grab/releases/latest/download/grab-darwin-amd64
chmod +x grab

# macOS Apple Silicon
curl -L -o grab https://github.com/vlln/grab/releases/latest/download/grab-darwin-arm64
chmod +x grab
```

### Build from source

Requires Go 1.23+.

```
go install github.com/vlln/grab@latest
```

Or clone and build:

```
git clone https://github.com/vlln/grab.git
cd grab
go build -o grab .
```

## Usage

```
grab [flags] <url>
```

| Flag | Default | Description |
|------|---------|-------------|
| `-o <file>` | stdout | Write body to file |
| `-H "K: V"` | — | Extra header (repeatable) |
| `--mode` | `auto` | `auto` → http → browser, `http` → http only, `browser` → browser only |
| `--timeout` | `30` | Request timeout in seconds |
| `--json` | false | Output JSON envelope |
| `-v` | false | Verbose logging to stderr |
| `-s` | false | Silent mode (curl-compatible) |
| `-A <ua>` | — | Custom User-Agent header |
| `--no-memory` | false | Skip domain memory |
| `--wait` | `0` | Extra seconds to wait after page load (browser mode) |

### Examples

```
grab https://example.com
grab -o page.html https://example.com
grab -H "Authorization: Bearer tok" https://api.example.com/data
grab --json https://example.com
grab -v --mode auto https://www.biorxiv.org/content/10.1101/2024.01.01.123456v1
```

### JSON output

```json
{
  "url": "https://example.com",
  "status_code": 200,
  "engine_used": "http",
  "fingerprint": "chrome_120",
  "from_memory": false,
  "headers": {"Content-Type": "text/html"},
  "body": "<!doctype html>..."
}
```

## How it works

1. **Memory cache** — If the domain was seen before, retry the fingerprint that worked last time.
2. **HTTP rotation** — Cycle through Chrome 120, Chrome 131, Safari 16.0 TLS fingerprints with matching headers.
3. **Browser fallback** — When HTTP fails, delegate to a [camofox](https://github.com/jo-inc/camofox-browser) browser instance for JS-heavy sites.

Memory is stored at `~/.grab/memory.json`. Use `--no-memory` to bypass it.

## Browser fallback (optional)

Browser mode requires [camofox](https://github.com/jo-inc/camofox-browser) running somewhere accessible:

```
# Start camofox (Docker)
docker run -d -p 9377:9377 camofox

# Use with grab
GRAB_CAMOFOX_URL=http://localhost:9377 grab --mode browser https://example.com
GRAB_CAMOFOX_URL=http://localhost:9377 grab --mode auto https://js-heavy-site.com
```

When `--mode auto`, grab only falls back to camofox after all HTTP attempts fail — so it works fine without it. Only `--mode browser` requires camofox to be configured.

## Real-world comparison

The table below compares **curl** (standard TLS + Chrome headers), **grab HTTP** (uTLS fingerprint rotation), and **grab Browser** (camofox) against 29 real websites, including academic publishers and CDN-protected sites.

### Academic publishers

| Site | curl | grab (HTTP) | grab (Browser) |
|------|------|-------------|----------------|
| biorxiv.org | 403 (Cloudflare) | **200** chrome_120 | — |
| sciencedirect.com | 403 (Cloudflare) | **200** chrome_120 | — |
| nejm.org | 403 (Cloudflare) | **200** chrome_120 | — |
| wiley.com | 403 (Cloudflare) | **200** chrome_131 | — |
| dl.acm.org | 403 (Cloudflare) | **200** chrome_120 | — |
| journals.plos.org | 000 (fail) | **200** chrome_120 | — |
| cell.com | 403 (Cloudflare) | 403 | **200** browser |
| thelancet.com | 403 (Cloudflare) | 302 (block) | **200** browser |
| medrxiv.org | 200 | 200 | — |
| pubmed.ncbi.nlm.nih.gov | 200 | 200 | — |
| nature.com | 200 | 200 | — |
| link.springer.com | 200 | 200 | — |
| ieeexplore.ieee.org | 200 | 200 | — |

**Result**: curl succeeds on 5/13 (38%). grab HTTP adds 6 more (85%). grab Browser covers the remaining 2 (100%).

### General sites

| Site | curl | grab (HTTP) | grab (Browser) |
|------|------|-------------|----------------|
| example.com | 200 | 200 chrome_120 | — |
| httpbin.org | 200 | 200 chrome_120 | — |
| wikipedia.org | 200 | 200 chrome_120 | — |
| github.com | 200 | 200 chrome_120 | — |
| arxiv.org | 200 | 200 chrome_120 | — |
| www.python.org | 200 | 200 chrome_120 | — |
| www.rust-lang.org | 200 | 200 chrome_120 | — |
| go.dev | 200 | 200 chrome_120 | — |
| www.ietf.org | 200 | 200 chrome_120 | — |
| cloudflare.com | 200 | 200 chrome_120 | — |
| stackoverflow.com | 403 (Cloudflare) | 000 (fail) | **200** browser |
| news.ycombinator.com | 200 | 000 (DNS) | **200** browser |
| reddit.com | 200 | 000 (timeout) | **200** browser |

**Result**: Most general sites work with both. Three edge cases fixed by browser fallback.

### How the layers stack

```
curl (standard TLS)      → blocked by Cloudflare JA3 fingerprinting
grab HTTP (uTLS rotate)  → bypasses fingerprint checks (85%+ of academic sites)
grab Browser (camofox)   → full Chromium rendering, bypasses everything (100%)
```

`--mode auto` (default) runs these layers in sequence: memory cache → HTTP rotation → browser fallback.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Runtime failure (all engines exhausted, network error, etc.) |
| 2 | Misconfiguration (invalid mode, `--mode browser` without camofox URL, etc.) |
## License

MIT
