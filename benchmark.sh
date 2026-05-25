#!/usr/bin/env bash
# Real-world comparison test: grab vs curl
# Tests TLS fingerprint rotation effectiveness against various sites.
set -euo pipefail

GRAB="/home/vlln/Project/grab/grab"
CURL="curl"
RESULTS_DIR="/tmp/grab-benchmark-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$RESULTS_DIR"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'
BOLD='\033[1m'; NC='\033[0m'

TARGETS=(
  "example|https://example.com|simple"
  "httpbin|https://httpbin.org/get|api"
  "httpbin-html|https://httpbin.org/html|simple"
  "wikipedia|https://en.wikipedia.org/wiki/Main_Page|simple"
  "github|https://github.com|cdn"
  "stackoverflow|https://stackoverflow.com|cdn"
  "biorxiv|https://www.biorxiv.org|academic"
  "nature|https://www.nature.com|academic"
  "reddit|https://www.reddit.com|cdn"
  "arxiv|https://arxiv.org|academic"
  "cloudflare-com|https://www.cloudflare.com|cdn"
  "python-org|https://www.python.org|simple"
  "rust-lang|https://www.rust-lang.org|cdn"
  "ietf|https://www.ietf.org|simple"
  "hackernews|https://news.ycombinator.com|simple"
  "go-dev|https://go.dev|simple"
)

TIMEOUT=25

# ── Detect block pages ──────────────────────────────────────────
check_block_page() {
  local file="$1"
  if [[ ! -s "$file" ]]; then echo "empty"; return; fi
  local content; content=$(head -c 5000 "$file" 2>/dev/null | tr '[:upper:]' '[:lower:]')
  if echo "$content" | grep -qi "cf-challenge\|cf-browser-verification\|just a moment\|attention required.*cloudflare"; then echo "cloudflare_block"; return; fi
  if echo "$content" | grep -qi "captcha\|verify you are human\|are you a robot|checking your browser"; then echo "captcha"; return; fi
  if echo "$content" | grep -qi "403 forbidden\|access denied\|error 403"; then echo "403"; return; fi
  if [[ "$content" == "" ]]; then echo "empty"; return; fi
  echo "ok"
}

# ── Test one URL with curl ──────────────────────────────────────
test_curl() {
  local name="$1" url="$2"
  local outfile="$RESULTS_DIR/${name}_curl_body.txt"
  local meta="$RESULTS_DIR/${name}_curl_meta.txt"
  local start elapsed http_code size blocked
  start=$(date +%s%N)
  http_code=$(curl -sS -o "$outfile" -w "%{http_code}" \
    --max-time "$TIMEOUT" -L \
    -H "User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36" \
    -H "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8" \
    -H "Accept-Language: en-US,en;q=0.9" \
    -D "$RESULTS_DIR/${name}_curl_headers.txt" \
    "$url" 2>/dev/null || echo "000")
  end=$(date +%s%N)
  elapsed=$(( (end - start) / 1000000 ))
  size=$(stat -c%s "$outfile" 2>/dev/null || echo 0)
  blocked=$(check_block_page "$outfile")
  echo "${http_code}|${elapsed}|${size}|${blocked}" > "$meta"
}

# ── Test one URL with grab ──────────────────────────────────────
# Uses --json to get metadata + body in one call (body embedded in JSON).
test_grab() {
  local name="$1" url="$2"
  local jsonfile="$RESULTS_DIR/${name}_grab.json"
  local bodyfile="$RESULTS_DIR/${name}_grab_body.txt"
  local meta="$RESULTS_DIR/${name}_grab_meta.txt"
  local start elapsed size blocked status_code fingerprint engine_used exit_code
  start=$(date +%s%N)
  exit_code=0
  "$GRAB" --mode http --timeout "$TIMEOUT" --json -s "$url" > "$jsonfile" 2>/dev/null || exit_code=$?
  end=$(date +%s%N)
  elapsed=$(( (end - start) / 1000000 ))

  status_code="000"; fingerprint="none"; engine_used="none"; size=0
  if [[ -s "$jsonfile" ]]; then
    status_code=$(jq -r '.status_code // "000"' "$jsonfile" 2>/dev/null || echo "000")
    fingerprint=$(jq -r '.fingerprint // "none"' "$jsonfile" 2>/dev/null || echo "none")
    engine_used=$(jq -r '.engine_used // "none"' "$jsonfile" 2>/dev/null || echo "none")
    # Extract body from JSON into separate file for block detection
    jq -r '.body // ""' "$jsonfile" 2>/dev/null > "$bodyfile"
    size=$(stat -c%s "$bodyfile" 2>/dev/null || echo 0)
  fi
  blocked=$(check_block_page "$bodyfile")
  echo "${exit_code}|${status_code}|${elapsed}|${size}|${fingerprint}|${engine_used}|${blocked}" > "$meta"
}

# ── Main ────────────────────────────────────────────────────────
echo -e "${BOLD}=== grab vs curl — Real-World Comparison Test ===${NC}"
echo "Date: $(date)"
echo "Results: $RESULTS_DIR"
echo -e "${CYAN}Testing ${#TARGETS[@]} URLs, timeout=${TIMEOUT}s...${NC}"
echo ""

# Run all in parallel: curl + grab for each site
pids=()
for entry in "${TARGETS[@]}"; do
  IFS='|' read -r name url category <<< "$entry"
  test_curl "$name" "$url" &
  pids+=($!)
  test_grab "$name" "$url" &
  pids+=($!)
done

for pid in "${pids[@]}"; do
  wait "$pid" 2>/dev/null || true
done

# ── Print results table ─────────────────────────────────────────
printf "${BOLD}%-18s %-6s %-9s | %-9s %-12s | %-8s %-8s | %-18s %-12s${NC}\n" \
  "Site" "Cat" "curl" "grab" "grab_FP" "curl_ms" "grab_ms" "curl_Block" "grab_Block"
printf '%s\n' "$(printf '%.0s-' {1..125})"

grab_wins=0; curl_wins=0; both_ok=0; both_fail=0
total_grab_ms=0; total_curl_ms=0; count=0
declare -A fp_stats fp_success
grab_advantage_domains=()

for entry in "${TARGETS[@]}"; do
  IFS='|' read -r name url category <<< "$entry"

  curl_meta=$(cat "$RESULTS_DIR/${name}_curl_meta.txt" 2>/dev/null || echo "000|0|0|error")
  IFS='|' read -r curl_code curl_ms curl_size curl_block <<< "$curl_meta"

  grab_meta=$(cat "$RESULTS_DIR/${name}_grab_meta.txt" 2>/dev/null || echo "0|000|0|0|error|error|error")
  IFS='|' read -r grab_exit grab_code grab_ms grab_size grab_fp grab_eng grab_block <<< "$grab_meta"

  # Determine "OK": 200 status AND body looks like real content
  curl_ok=false; grab_ok=false
  [[ "$curl_code" == "200" ]] && [[ "$curl_block" == "ok" ]] && curl_ok=true
  [[ "$grab_code" == "200" ]] && [[ "$grab_block" == "ok" ]] && grab_ok=true

  if $curl_ok && $grab_ok; then both_ok=$((both_ok + 1));
  elif $curl_ok && ! $grab_ok; then curl_wins=$((curl_wins + 1));
  elif ! $curl_ok && $grab_ok; then grab_wins=$((grab_wins + 1));
  else both_fail=$((both_fail + 1)); fi

  total_curl_ms=$((total_curl_ms + curl_ms))
  total_grab_ms=$((total_grab_ms + grab_ms))
  count=$((count + 1))

  fp_key="${grab_fp:-none}"
  fp_stats["$fp_key"]=$(( ${fp_stats["$fp_key"]:-0} + 1 ))
  $grab_ok && fp_success["$fp_key"]=$(( ${fp_success["$fp_key"]:-0} + 1 ))

  if ! $curl_ok && $grab_ok; then
    grab_advantage_domains+=("$name|$url|$grab_fp|$curl_block")
  fi

  # Format status columns
  if $curl_ok; then c_icon="${GREEN}200 OK${NC}  "; else c_icon="${RED}$(printf '%-6s' "$curl_code")${NC}"; fi
  if $grab_ok; then g_icon="${GREEN}200 OK${NC}  "; else g_icon="${RED}$(printf '%-6s' "$grab_code")${NC}"; fi

  if [[ "$curl_block" != "ok" ]]; then cb_icon="${YELLOW}$(printf '%-16s' "$curl_block")${NC}"; else cb_icon="$(printf '%-16s' '-')"; fi
  if [[ "$grab_block" != "ok" ]]; then gb_icon="${YELLOW}$(printf '%-12s' "$grab_block")${NC}"; else gb_icon="$(printf '%-12s' '-')"; fi

  printf "%-18s %-6s %-9s | %-9s %-12s | %-8s %-8s | %-18s %-12s\n" \
    "$name" "$category" "$c_icon" "$g_icon" "${grab_fp:-none}" "${curl_ms}ms" "${grab_ms}ms" "$cb_icon" "$gb_icon"
done

# ── Summary ─────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}════════════════════════════════════════════════${NC}"
echo -e "${BOLD}               RESULTS SUMMARY${NC}"
echo -e "${BOLD}════════════════════════════════════════════════${NC}"
echo ""
echo -e "  Both OK:        ${GREEN}${both_ok}${NC}"
echo -e "  curl only OK:   ${YELLOW}${curl_wins}${NC}"
echo -e "  grab only OK:   ${GREEN}${grab_wins}${NC}"
echo -e "  Both failed:    ${RED}${both_fail}${NC}"
echo ""

if [[ $count -gt 0 ]]; then
  avg_curl=$((total_curl_ms / count))
  avg_grab=$((total_grab_ms / count))
  echo -e "  avg curl time:  ${avg_curl}ms"
  echo -e "  avg grab time:  ${avg_grab}ms"
  if [[ $avg_grab -lt $avg_curl ]]; then
    echo -e "  grab is ${GREEN}$(( (avg_curl - avg_grab) * 100 / avg_curl ))% faster${NC} on average"
  elif [[ $avg_grab -gt $avg_curl ]]; then
    echo -e "  curl is $(( (avg_grab - avg_curl) * 100 / avg_grab ))% faster on average"
  fi
fi

echo ""
echo -e "${BOLD}Fingerprint distribution:${NC}"
for fp in "${!fp_stats[@]}"; do
  succ="${fp_success[$fp]:-0}"
  echo "  $fp: ${fp_stats[$fp]} attempts, ${succ} success"
done | sort -t: -k2 -rn

echo ""
echo -e "${BOLD}Key insight — curl blocked, grab succeeded:${NC}"
if [[ ${#grab_advantage_domains[@]} -eq 0 ]]; then
  echo "  (none found in this run)"
else
  for entry in "${grab_advantage_domains[@]}"; do
    IFS='|' read -r d url fp block <<< "$entry"
    echo -e "  ${GREEN}${d}${NC}: curl=${block}, grab=${fp} → ${url}"
  done
fi

echo ""
echo -e "Full results: ${CYAN}${RESULTS_DIR}${NC}"