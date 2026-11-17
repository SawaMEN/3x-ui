#!/usr/bin/env bash
# Determine which CI suites are required from the paths changed in one commit/PR.
set -euo pipefail
if [[ $# != 1 || ! -r ${1:-} ]]; then
  echo "usage: $0 FILE_WITH_CHANGED_PATHS" >&2
  exit 2
fi
backend=false
frontend=false
while IFS= read -r path || [[ -n "$path" ]]; do
  [[ -n "$path" ]] || continue
  case "$path" in
    frontend/*|.nvmrc)
      frontend=true ;;
    *.go|go.mod|go.sum|Makefile|deploy/*|internal/*|scripts/*|tools/*|install.sh|update.sh|.github/workflows/*)
      backend=true ;;
    *)
      # Unrecognized paths fail open: test both suites rather than skip checks.
      backend=true
      frontend=true ;;
  esac
done < "$1"
# A zero-change diff may reflect a Git merge or shallow checkout: fail open.
if [[ "$backend" == false && "$frontend" == false ]]; then
  backend=true
  frontend=true
fi
printf 'backend=%s\nfrontend=%s\n' "$backend" "$frontend" >> "${GITHUB_OUTPUT:-/dev/stdout}"
