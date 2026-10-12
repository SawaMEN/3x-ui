#!/usr/bin/env bash
set -euo pipefail

# Retry dependency transfer before testing. Test failures are never retried.
for attempt in 1 2 3; do
    if go mod download; then
        exit 0
    fi
    if [[ "$attempt" == 3 ]]; then
        echo "Go dependency download failed after three attempts" >&2
        exit 1
    fi
    sleep "$((attempt * 5))"
done
