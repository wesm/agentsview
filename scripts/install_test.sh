#!/bin/bash
# Tests for install.sh version parsing logic.
# Sources install.sh directly so the test exercises the real
# get_latest_version function (with curl mocked out).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source install.sh to get access to get_latest_version.
# The main() call is guarded so nothing runs on source.
# shellcheck source=install.sh
source "$SCRIPT_DIR/install.sh"

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

# Mock curl to return a fixture instead of hitting the network.
# get_latest_version uses `curl -fsSL "$url"` so we intercept
# that and emit $MOCK_JSON.
MOCK_JSON=""
curl() { printf '%s\n' "$MOCK_JSON"; }
export -f curl

echo "=== get_latest_version parsing ==="

# Pretty-printed JSON (typical curl response)
MOCK_JSON='{
  "url": "https://api.github.com/repos/wesm/agentsview/releases/291105519",
  "tag_name": "v0.8.0",
  "name": "v0.8.0"
}'
assert_eq "pretty-printed JSON" "v0.8.0" "$(get_latest_version)"

# Minified JSON (the case that caused #61)
MOCK_JSON='{"url":"https://api.github.com/repos/wesm/agentsview/releases/291105519","assets_url":"https://api.github.com/repos/wesm/agentsview/releases/291105519/assets","tag_name":"v0.8.0","name":"v0.8.0"}'
assert_eq "minified JSON" "v0.8.0" "$(get_latest_version)"

# tag_name before url field
MOCK_JSON='{"tag_name":"v1.2.3","url":"https://api.github.com/repos/wesm/agentsview/releases/1"}'
assert_eq "tag_name before url" "v1.2.3" "$(get_latest_version)"

# Extra whitespace around colon
MOCK_JSON='{  "tag_name" :  "v2.0.0"  }'
assert_eq "extra whitespace" "v2.0.0" "$(get_latest_version)"

# Pre-release version
MOCK_JSON='{"tag_name":"v0.9.0-rc1","name":"v0.9.0-rc1"}'
assert_eq "pre-release version" "v0.9.0-rc1" "$(get_latest_version)"

# No tag_name field (API error / rate limit)
MOCK_JSON='{"message":"API rate limit exceeded"}'
assert_eq "missing tag_name returns empty" "" "$(get_latest_version)"

echo
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
