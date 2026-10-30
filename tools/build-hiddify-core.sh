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
print(lock['target']['version'])
print(lock['target']['commit'])
print(','.join(lock['panel_build']['tags']))
print(lock['panel_build']['sing_box_version'])
print(lock['panel_build']['toolchain'])
print(lock['target']['submodules']['hiddify-sing-box'])
PY
)
if [[ ${#target[@]} != 7 ]]; then
  printf '%s\n' 'Failed to read pinned target manifest' >&2
  exit 1
fi
git init -q "$task_source/core"
git -C "$task_source/core" remote add origin "${target[0]}"
git -C "$task_source/core" fetch --depth=1 origin "${target[2]}"
git -C "$task_source/core" checkout -q --detach FETCH_HEAD
git -C "$task_source/core" config submodule.hiddify-sing-box.url https://github.com/SawaMEN/hiddify-sing-box.git
git -C "$task_source/core" config submodule.ray2sing.url https://github.com/hiddify/ray2sing.git
git -C "$task_source/core" submodule update --init --depth=1
# Track the independently maintained Sing-box fork even if the parent's
# gitlink has not advanced. The lock retains the exact fetched revision.
git -C "$task_source/core/hiddify-sing-box" fetch --depth=1 origin "${target[6]}"
git -C "$task_source/core/hiddify-sing-box" checkout -q --detach FETCH_HEAD
git -C "$task_source/core/hiddify-sing-box" submodule update --init --recursive --depth=1
python3 "$task_root/tools/audit-hiddify-migration.py" --upstream "$task_source/core"
git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/speed/masque.patch"
mkdir -p "$task_source/core/hiddify-sing-box/internal/xuispeed"
cp "$task_root/internal/clientlimit/brokerclient/client.go" "$task_source/core/hiddify-sing-box/internal/xuispeed/client.go"

git -C "$task_source/core" apply "$task_root/deploy/hiddify/runtime-dependencies.patch"
(
  cd -- "$task_source/core"
  GOTOOLCHAIN="${target[5]}" go mod edit -go="${target[5]#go}" -require=golang.org/x/net@v0.60.0
  GOTOOLCHAIN="${target[5]}" go mod download golang.org/x/net@v0.60.0
)
git -C "$task_source/core" apply "$task_root/deploy/hiddify/clash-user.patch"
git -C "$task_source/core" apply "$task_root/deploy/hiddify/sniff-override.patch"
git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/http1-options.patch"
git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/inbound-finalmask.patch"
cp -- "$task_root/deploy/hiddify/inbound_finalmask_test.go.in" "$task_source/core/hiddify-sing-box/xui_inbound_finalmask_test.go"
cp -- "$task_root/deploy/hiddify/http1_options_test.go.in" "$task_source/core/hiddify-sing-box/option/xui_panel_http1_test.go"
cp -- "$task_root/deploy/hiddify/cmd_xui_check.go.in" "$task_source/core/cmd/cmd_xui_check.go"
cp -- "$task_root/deploy/hiddify/clash_user_test.go.in" "$task_source/core/hiddify-sing-box/experimental/clashapi/trafficontrol/xui_panel_test.go"

cp -- "$task_root/deploy/hiddify/sniff_override_test.go.in" "$task_source/core/hiddify-sing-box/route/rule/xui_panel_sniff_test.go"

# Modify verified dependency copies, never the shared Go module cache.
task_module_cache=$(go env GOMODCACHE)
(
  cd -- "$task_source/core"
  GOTOOLCHAIN="${target[5]}" go mod download github.com/metacubex/utls@v1.8.8 github.com/starifly/sing-vmess@v0.2.7-mod.9 github.com/xtls/xray-core@v1.260327.1-0.20260728075948-5ca6f4b7d4dc
)
mkdir -p "$task_source/core/xui-dependencies"
cp -a "$task_module_cache/github.com/metacubex/utls@v1.8.8" "$task_source/core/xui-dependencies/utls"
cp -a "$task_module_cache/github.com/starifly/sing-vmess@v0.2.7-mod.9" "$task_source/core/xui-dependencies/sing-vmess"
cp -a "$task_module_cache/github.com/xtls/xray-core@v1.260327.1-0.20260728075948-5ca6f4b7d4dc" "$task_source/core/xui-dependencies/xray-core"
chmod -R u+w "$task_source/core/xui-dependencies"
git -C "$task_source/core" apply --directory=xui-dependencies/utls "$task_root/deploy/hiddify/utls-protection.patch"
git -C "$task_source/core" apply --directory=xui-dependencies/sing-vmess "$task_root/deploy/hiddify/sing-vmess-protection.patch"
git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/protection-options.patch"
git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/transport-balancer.patch"
git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/vless-reverse.patch"
git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/vless-handshake.patch"
cp "$task_root/deploy/hiddify/vless_handshake_test.go.in" "$task_source/core/hiddify-sing-box/protocol/vless/xui_handshake_test.go"
git -C "$task_source/core" apply --directory=xui-dependencies/sing-vmess "$task_root/deploy/hiddify/sing-vmess-reverse.patch"
cp "$task_root/deploy/hiddify/reverse_vision_test.go.in" "$task_source/core/xui-dependencies/sing-vmess/vless/xui_reverse_test.go"
cp "$task_root/deploy/hiddify/reverse_runtime_test.go.in" "$task_source/core/hiddify-sing-box/protocol/vless/xui_reverse_test.go"
git -C "$task_source/core" apply --directory=xui-dependencies/xray-core "$task_root/deploy/hiddify/xray-runtime-compat.patch"
cp "$task_root/deploy/hiddify/kcp_transport_test.go.in" "$task_source/core/hiddify-sing-box/transport/v2raykcp/xui_kcp_test.go"
cp "$task_root/deploy/hiddify/xhttp_extension_test.go.in" "$task_source/core/hiddify-sing-box/transport/v2rayxhttp/xui_xhttp_test.go"
cp "$task_root/deploy/hiddify/balancer_strategy_test.go.in" "$task_source/core/hiddify-sing-box/protocol/group/xui_balancer_test.go"
cp "$task_root/deploy/hiddify/reality_protection_test.go.in" "$task_source/core/hiddify-sing-box/common/tls/xui_panel_reality_test.go"
cp "$task_root/deploy/hiddify/vision_preconnect_test.go.in" "$task_source/core/hiddify-sing-box/protocol/vless/xui_preconnect_test.go"
cp "$task_root/deploy/hiddify/vision_padding_test.go.in" "$task_source/core/xui-dependencies/sing-vmess/vless/xui_vision_options_test.go"
(
  cd -- "$task_source/core"
  GOTOOLCHAIN="${target[5]}" go mod edit -replace github.com/metacubex/utls=./xui-dependencies/utls -replace github.com/sagernet/sing-vmess=./xui-dependencies/sing-vmess
  GOTOOLCHAIN="${target[5]}" go mod edit -replace github.com/xtls/xray-core=./xui-dependencies/xray-core
  # Reconcile the module graph after applying the pinned fork and overlays.
  GOTOOLCHAIN="${target[5]}" go mod tidy
)

task_goos=${GOOS:-$(go env GOOS)}
task_goarch=${GOARCH:-$(go env GOARCH)}
task_binary="hiddify-core-$task_goos-$task_goarch"
task_cgo=0
task_link_flags="-s -w -checklinkname=0 -X github.com/hiddify/hiddify-core/v2/hcommon/constants.Version=${target[1]} -X github.com/sagernet/sing-box/constant.Version=${target[4]}"
if [[ ${SINGBOX_CRONET_LINKAGE:-purego} == musl ]]; then
  [[ "$task_goos" == linux ]] || { printf '%s\n' 'musl linkage requires Linux' >&2; exit 1; }
  task_cgo=1
  command -v ld.lld >/dev/null || { printf '%s\n' 'musl Cronet linkage requires LLVM LLD' >&2; exit 1; }
  command -v readelf >/dev/null || { printf '%s\n' 'musl linkage verification requires readelf' >&2; exit 1; }
  task_link_flags+=' -linkmode external -extldflags "-static -fuse-ld=lld"'
  target[3]=${target[3]/with_purego/with_musl}
elif [[ ${SINGBOX_CRONET_LINKAGE:-purego} != purego ]]; then
  printf '%s\n' 'Unknown Cronet linkage profile' >&2; exit 1
fi
if [[ "$task_goos" == windows ]]; then
  task_binary="$task_binary.exe"
fi
if [[ "$task_goos" == "$(go env GOHOSTOS)" && "$task_goarch" == "$(go env GOHOSTARCH)" ]]; then
  (
    cd -- "$task_source/core"
    CGO_ENABLED="$task_cgo" GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_link_flags" -tags "${target[3]}" github.com/sagernet/sing-box/experimental/clashapi/trafficontrol -run '^TestXUIPanelAuthenticatedUserIsConnectionMetadata$' -count=1
    CGO_ENABLED="$task_cgo" GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_link_flags" -tags "${target[3]}" github.com/sagernet/sing-box -run '^TestXUIPanelInboundFinalMask' -count=1 -timeout=60s
    CGO_ENABLED="$task_cgo" GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_link_flags" -tags "${target[3]}" github.com/sagernet/sing-box/transport/v2rayxdrive -count=1 -timeout=90s
    CGO_ENABLED="$task_cgo" GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_link_flags" -tags "${target[3]}" github.com/sagernet/sing-box/protocol/snell -run '^TestSnellV[56]' -count=1 -timeout=60s
    CGO_ENABLED="$task_cgo" GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_link_flags" -tags "${target[3]}" github.com/sagernet/sing-box/option -run '^TestXUIPanelHTTP1RoundTrip$' -count=1
    CGO_ENABLED="$task_cgo" GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_link_flags" -tags "${target[3]}" github.com/sagernet/sing-box/route/rule -run '^TestXUIPanelSniffOverrideAction$' -count=1
    CGO_ENABLED="$task_cgo" GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_link_flags" -tags "${target[3]}" github.com/sagernet/sing-box/common/tls github.com/sagernet/sing-box/protocol/vless github.com/sagernet/sing-vmess/vless -run '^TestXUIPanel(Reality|Preconnect|Vision|Reverse|Encryption)' -count=1 -timeout=60s
    CGO_ENABLED="$task_cgo" GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_link_flags" -tags "${target[3]}" github.com/sagernet/sing-box/transport/v2raykcp github.com/sagernet/sing-box/transport/v2rayxhttp github.com/sagernet/sing-box/protocol/group -run '^TestXUIPanel' -count=1 -timeout=90s
  )
fi
(
  cd -- "$task_source/core"
  CGO_ENABLED="$task_cgo" GOTOOLCHAIN="${target[5]}" go build -buildvcs=true -trimpath -tags "${target[3]}" \
    -ldflags "$task_link_flags" \
    -o "$task_output/$task_binary" ./cmd/main
)
if [[ ${SINGBOX_CRONET_LINKAGE:-purego} == musl ]]; then
  task_elf_programs=$(readelf -l "$task_output/$task_binary")
  task_elf_dynamic=$(readelf -d "$task_output/$task_binary")
  if grep -q INTERP <<< "$task_elf_programs" || grep -q NEEDED <<< "$task_elf_dynamic"; then
    printf '%s\n' 'musl kernel must have no ELF interpreter or shared-library dependencies' >&2
    exit 1
  fi
fi
if [[ "$task_goos" == "$(go env GOHOSTOS)" && "$task_goarch" == "$(go env GOHOSTARCH)" ]]; then
  task_version=$("$task_output/$task_binary" version)
  printf '%s\n' "$task_version"
  if ! [[ "$task_version" == *"hiddify-core version ${target[1]} "* && "$task_version" == *"Revision: ${target[2]}"* ]]; then
    printf '%s\n' 'Built core identity differs from pinned source' >&2
    exit 1
  fi
  printf '%s\n' '{"experimental":{"v2ray_api":{"listen":"127.0.0.1:0","stats":{"enabled":true,"users":["build-check"]}}}}' > "$task_source/check.json"
  "$task_output/$task_binary" check -c "$task_source/check.json"
  if "$task_output/$task_binary" check > /dev/null 2>&1; then
    printf '%s\n' 'Built core accepts check without a candidate config' >&2
    exit 1
  fi
fi
python3 - "$task_root/docs/hiddify-core-target.json" "$task_output/target.json" "${target[3]}" "${SINGBOX_CRONET_LINKAGE:-purego}" <<'PYMANIFEST'
import json, sys
lock=json.load(open(sys.argv[1]))
lock['panel_build']['tags']=sys.argv[3].split(',')
lock['panel_build']['cronet_linkage']=sys.argv[4]
if sys.argv[4]=='musl': lock['panel_build']['runtime_dependencies']=[]
with open(sys.argv[2],'w') as output: json.dump(lock,output,indent=2); output.write('\n')
PYMANIFEST
(
  cd -- "$task_output"
  sha256sum "$task_binary" > "$task_binary.sha256"
)
printf 'Built %s\n' "$task_output/$task_binary"
if [[ "$task_cgo" == 0 ]]; then
  task_cronet_revision=$(tr -d '\r\n' < "$task_source/core/hiddify-sing-box/.github/CRONET_GO_VERSION")
  task_library=libcronet.so
  [[ "$task_goos" != windows ]] || task_library=libcronet.dll
  [[ "$task_goos" != darwin ]] || task_library=libcronet.dylib
  task_platform="${task_goos}_${task_goarch}"
  git init -q "$task_source/cronet"
  git -C "$task_source/cronet" remote add origin https://github.com/sagernet/cronet-go.git
  git -C "$task_source/cronet" sparse-checkout set --no-cone "/lib/$task_platform/$task_library"
  git -C "$task_source/cronet" fetch --depth=1 --filter=blob:none origin "$task_cronet_revision"
  git -C "$task_source/cronet" checkout -q --detach FETCH_HEAD
  cp -- "$task_source/cronet/lib/$task_platform/$task_library" "$task_output/$task_library"
  (cd -- "$task_output"; sha256sum "$task_library" > "$task_library.sha256")
fi

if [[ "$task_goos" == "$(go env GOHOSTOS)" && "$task_goarch" == "$(go env GOHOSTARCH)" ]]; then
  printf '%s\n' '{"outbounds":[{"type":"naive","tag":"cronet-check","server":"127.0.0.1","server_port":443,"username":"build-check","password":"build-check","tls":{"enabled":true}}]}' > "$task_source/naive-check.json"
  "$task_output/$task_binary" check -c "$task_source/naive-check.json"
fi
