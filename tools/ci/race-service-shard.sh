#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 || ! $1 =~ ^[0-9]+$ || ! $2 =~ ^[1-9][0-9]*$ ]] || (( $1 >= $2 )); then
  echo "usage: $0 SHARD_INDEX SHARD_COUNT [--list]" >&2
  exit 2
fi
if [[ $# -eq 3 && $3 != --list ]]; then
  echo "usage: $0 SHARD_INDEX SHARD_COUNT [--list]" >&2
  exit 2
fi

shard=$1
count=$2
test_list=$(mktemp)
trap 'rm -f "$test_list"' EXIT

# Go's list output contains the top-level tests, examples and fuzz seed tests.
# Match only names, not TestMain's setup output or the final package summary.
go test -race -short -vet=off -list . ./internal/web/service > "$test_list"
mapfile -t names < <(grep -E '^(Test|Example|Fuzz)[[:alnum:]_]+$' "$test_list" || true)
if (( ${#names[@]} == 0 )); then
  echo "no service tests found" >&2
  exit 1
fi

selected=()
for (( i=shard; i<${#names[@]}; i+=count )); do
  selected+=("${names[i]}")
done
if (( ${#selected[@]} == 0 )); then
  echo "shard $shard/$count has no tests" >&2
  exit 1
fi

if [[ ${3:-} == --list ]]; then
  printf '%s\n' "${selected[@]}"
  exit 0
fi

printf 'Running service race shard %d/%d (%d of %d top-level tests)\n' \
  "$shard" "$count" "${#selected[@]}" "${#names[@]}"
pattern="^($(IFS='|'; echo "${selected[*]}"))$"
go test -race -short -vet=off -timeout 20m -run "$pattern" ./internal/web/service
