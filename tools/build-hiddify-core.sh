#!/usr/bin/env bash
set -euo pipefail

task_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
task_output=${1:-"$task_root/bin/hiddify-core"}
mkdir -p -- "$task_output"
task_output=$(cd -- "$task_output" && pwd)
task_source=$(mktemp -d)
trap 'rm -rf -- "$task_source"' EXIT

readarray -t target < <(python3 - "$task_root/docs/hiddify-core-target.json" <<'PY'
import json, sys
lock = json.load(open(sys.argv[1]))
print(lock['target']['repository'])
print(lock['target']['tag'])
print(lock['target']['commit'])
print(','.join(lock['panel_build']['tags']))
print(lock['panel_build']['sing_box_version'])
print(lock['panel_build']['toolchain'])
PY
)
if [[ ${#target[@]} != 6 ]]; then
  printf '%s\n' 'Failed to read pinned target manifest' >&2
  exit 1
fi
git clone --depth=1 --branch "${target[1]}" -- "${target[0]}" "$task_source/core"
git -C "$task_source/core" config submodule.ray2sing.url https://github.com/hiddify/ray2sing.git
git -C "$task_source/core" submodule update --init --recursive --depth=1
python3 "$task_root/tools/audit-hiddify-migration.py" --upstream "$task_source/core"
git -C "$task_source/core" apply --unidiff-zero "$task_root/deploy/hiddify/clash-user.patch"
cp -- "$task_root/deploy/hiddify/cmd_xui_check.go.in" "$task_source/core/cmd/cmd_xui_check.go"

task_goos=${GOOS:-$(go env GOOS)}
task_goarch=${GOARCH:-$(go env GOARCH)}
task_binary="hiddify-core-$task_goos-$task_goarch"
if [[ "$task_goos" == windows ]]; then
  task_binary="$task_binary.exe"
fi
(
  cd -- "$task_source/core"
  CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go build -trimpath -tags "${target[3]}" \
    -ldflags "-s -w -checklinkname=0 -X github.com/hiddify/hiddify-core/v2/hcommon/constants.Version=${target[1]} -X github.com/sagernet/sing-box/constant.Version=${target[4]}" \
    -o "$task_output/$task_binary" ./cmd/main
)
if [[ "$task_goos" == "$(go env GOHOSTOS)" && "$task_goarch" == "$(go env GOHOSTARCH)" ]]; then
  "$task_output/$task_binary" version
  printf '%s\n' '{"experimental":{"v2ray_api":{"listen":"127.0.0.1:0","stats":{"enabled":true,"users":["build-check"]}}}}' > "$task_source/check.json"
  "$task_output/$task_binary" check -c "$task_source/check.json"
fi
cp -- "$task_root/docs/hiddify-core-target.json" "$task_output/target.json"
(
  cd -- "$task_output"
  sha256sum "$task_binary" > "$task_binary.sha256"
)
printf 'Built %s\n' "$task_output/$task_binary"
printf '%s\n' 'Naive outbound additionally requires the matching libcronet runtime on the target server.'
