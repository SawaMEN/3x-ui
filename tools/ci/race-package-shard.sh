#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 || $# -gt 4 || ! $2 =~ ^[0-9]+$ || ! $3 =~ ^[1-9][0-9]*$ ]] || (( $2 >= $3 )); then
  echo "usage: $0 PACKAGE SHARD_INDEX SHARD_COUNT [--list]" >&2
  exit 2
fi
if [[ $# -eq 4 && $4 != --list ]]; then
  echo "usage: $0 PACKAGE SHARD_INDEX SHARD_COUNT [--list]" >&2
  exit 2
fi

package=$1
shard=$2
count=$3
test_list=$(mktemp)
trap 'rm -f "$test_list"' EXIT

# Go's list output contains the top-level tests, examples and fuzz seed tests.
# Match only names, not TestMain's setup output or the final package summary.
go test -race -short -vet=off -list . "$package" > "$test_list"
mapfile -t names < <(grep -E '^(Test|Example|Fuzz)[[:alnum:]_]+$' "$test_list" || true)
if (( ${#names[@]} == 0 )); then
  echo "no tests found in $package" >&2
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

if [[ ${4:-} == --list ]]; then
  printf '%s\n' "${selected[@]}"
  exit 0
fi

printf 'Running %s race shard %d/%d (%d of %d top-level tests)\n' \
  "$package" "$shard" "$count" "${#selected[@]}" "${#names[@]}"
pattern="^($(IFS='|'; echo "${selected[*]}"))$"
go test -race -short -vet=off -timeout 20m -run "$pattern" "$package"
