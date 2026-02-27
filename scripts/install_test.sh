#!/bin/bash
# Tests for install.sh version parsing logic
set -euo pipefail

PASS=0
FAIL=0

assert_eq() {
    local desc="$1" expected="$2" actual="$3"
    if [ "$expected" = "$actual" ]; then
        echo "  PASS: $desc"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $desc"
        echo "    expected: '$expected'"
        echo "    actual:   '$actual'"
        FAIL=$((FAIL + 1))
    fi
}

parse_tag_name() {
    echo "$1" \
        | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' \
        | head -1 \
        | cut -d'"' -f4
}

echo "=== get_latest_version parsing ==="

# Pretty-printed JSON (typical curl response)
PRETTY='{
  "url": "https://api.github.com/repos/wesm/agentsview/releases/291105519",
  "tag_name": "v0.8.0",
  "name": "v0.8.0"
}'
assert_eq "pretty-printed JSON" "v0.8.0" "$(parse_tag_name "$PRETTY")"

# Minified JSON (the case that caused #61)
MINIFIED='{"url":"https://api.github.com/repos/wesm/agentsview/releases/291105519","assets_url":"https://api.github.com/repos/wesm/agentsview/releases/291105519/assets","tag_name":"v0.8.0","name":"v0.8.0"}'
assert_eq "minified JSON" "v0.8.0" "$(parse_tag_name "$MINIFIED")"

# tag_name before url field
REORDERED='{"tag_name":"v1.2.3","url":"https://api.github.com/repos/wesm/agentsview/releases/1"}'
assert_eq "tag_name before url" "v1.2.3" "$(parse_tag_name "$REORDERED")"

# Extra whitespace around colon
SPACED='{  "tag_name" :  "v2.0.0"  }'
assert_eq "extra whitespace" "v2.0.0" "$(parse_tag_name "$SPACED")"

# Pre-release version
PRERELEASE='{"tag_name":"v0.9.0-rc1","name":"v0.9.0-rc1"}'
assert_eq "pre-release version" "v0.9.0-rc1" "$(parse_tag_name "$PRERELEASE")"

# No tag_name field (API error / rate limit)
NO_TAG='{"message":"API rate limit exceeded"}'
assert_eq "missing tag_name returns empty" "" "$(parse_tag_name "$NO_TAG")"

echo
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
