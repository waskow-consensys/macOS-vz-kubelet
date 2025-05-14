#!/usr/bin/env bash
# sign-and-run.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

binary="$1"
shift

# 1) codesign the test binary in place
codesign --entitlements "$SCRIPT_DIR/../../resources/vz.entitlements" -s - "$binary" \
|| exit 1

# 2) exec into it with the original args
exec "$binary" "$@"
