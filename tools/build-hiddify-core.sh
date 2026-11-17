#!/usr/bin/env bash
set -euo pipefail

task_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
task_output=${1:-"$task_root/bin/hiddify-core"}
mkdir -p -- "$task_output"
task_output=$(cd -- "$task_output" && pwd)
task_source=$(mktemp -d)
trap 'rm -rf -- "$task_source"' EXIT

readarray -t target < <(python3 - "$task_root/deploy/hiddify/hiddify-core-target.json" <<'PY'
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
git -C "$task_source/core" config submodule.ray2sing.url https://github.com/SawaMEN/ray2sing.git
git -C "$task_source/core" submodule update --init --depth=1
# Track the independently maintained Sing-box fork even if the parent's
# gitlink has not advanced. The lock retains the exact fetched revision.
git -C "$task_source/core/hiddify-sing-box" fetch --depth=1 origin "${target[6]}"
git -C "$task_source/core/hiddify-sing-box" checkout -q --detach FETCH_HEAD
git -C "$task_source/core/hiddify-sing-box" submodule update --init --recursive --depth=1
python3 "$task_root/tools/verify-hiddify-build.py" --upstream "$task_source/core"
# The release only uses features that are already tracked in the pinned
# Sing-box and core repositories. Native files are checked from the lock.
python3 - "$task_root/deploy/hiddify/hiddify-core-target.json" "$task_source/core" <<'PYNATIVE'
import json, pathlib, sys
lock = json.load(open(sys.argv[1]))
root = pathlib.Path(sys.argv[2])
sources = lock['panel_build']['native_source_files']
assert len(sources) == len(set(sources)) and len(sources) >= 20, "native feature list incomplete"
for path in sources:
    candidate = root / path
    if not candidate.is_file():
        raise SystemExit(f"Missing reviewed native source: {path}")
PYNATIVE

(
  cd -- "$task_source/core"
  readarray -t security_dependencies < <(python3 - "$task_root/deploy/hiddify/hiddify-core-target.json" <<'PYDEPS'
import json, sys
for module, version in sorted(json.load(open(sys.argv[1]))['panel_build']['security_dependencies'].items()):
    print(module + '@' + version)
PYDEPS
  )
  ((${#security_dependencies[@]} > 0)) || { echo 'Missing security dependency pins' >&2; exit 1; }
  security_flags=()
  for dependency in "${security_dependencies[@]}"; do security_flags+=("-require=$dependency"); done
  GOTOOLCHAIN="${target[5]}" go mod edit -go="${target[5]#go}" "${security_flags[@]}"
  GOTOOLCHAIN="${target[5]}" go mod download "${security_dependencies[@]}"
)
# Reconcile pinned patched dependencies without overlay copies or external patches.
(
  cd -- "$task_source/core"
  GOTOOLCHAIN="${target[5]}" go mod tidy
)

task_goos=${GOOS:-$(go env GOOS)}
task_goarch=${GOARCH:-$(go env GOARCH)}
task_binary="hiddify-core-$task_goos-$task_goarch"
task_cgo=0
task_link_flags="-s -w -checklinkname=0 -X github.com/hiddify/hiddify-core/v2/hcommon/constants.Version=${target[1]} -X github.com/sagernet/sing-box/constant.Version=${target[4]}"
# Run protocol regression tests with the non-CGO Cronet backend. The release
# executable is still compiled and smoke-tested with the requested musl profile.
# Statically linking the Go test harness against musl crashes before the tests
# run on both amd64 and arm64 release builders.
task_test_tags=${target[3]}
task_test_link_flags=$task_link_flags
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
    CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_test_link_flags" -tags "$task_test_tags" github.com/sagernet/sing-box/transport/v2rayhttpupgrade github.com/sagernet/sing-box/transport/trojan -run '^TestXUIPanel' -count=1 -timeout=30s
    CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_test_link_flags" -tags "$task_test_tags" github.com/sagernet/sing-box/experimental/clashapi/trafficontrol -run '^TestXUIPanelAuthenticatedUserIsConnectionMetadata$' -count=1
    CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_test_link_flags" -tags "$task_test_tags" github.com/sagernet/sing-box -run '^TestXUIPanelInboundFinalMask' -count=1 -timeout=60s
    CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_test_link_flags" -tags "$task_test_tags" github.com/sagernet/sing-box/transport/v2rayxdrive -count=1 -timeout=90s
    CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_test_link_flags" -tags "$task_test_tags" github.com/sagernet/sing-box/protocol/snell -run '^TestSnellV[56]' -count=1 -timeout=60s
    CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_test_link_flags" -tags "$task_test_tags" github.com/sagernet/sing-box/option -run '^TestXUIPanelHTTP1RoundTrip$' -count=1
    CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_test_link_flags" -tags "$task_test_tags" github.com/sagernet/sing-box/route/rule -run '^TestXUIPanelSniffOverrideAction$' -count=1
    CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_test_link_flags" -tags "$task_test_tags" github.com/sagernet/sing-box/common/tls github.com/sagernet/sing-box/protocol/vless github.com/sagernet/sing-vmess/vless -run '^TestXUIPanel(Reality|Preconnect|Vision|Reverse|Encryption|ExplicitCipher)' -count=1 -timeout=60s
    CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_test_link_flags" -tags "$task_test_tags" github.com/sagernet/sing-box/transport/v2raykcp github.com/sagernet/sing-box/transport/v2rayxhttp github.com/sagernet/sing-box/protocol/group -run '^TestXUIPanel' -count=1 -timeout=90s
  )
fi
# Regression coverage lives in the pinned native source tree, not in copied overlays.
if [[ "$task_goos" == "$(go env GOHOSTOS)" && "$task_goarch" == "$(go env GOHOSTARCH)" ]]; then
  (
    cd -- "$task_source/core"
    CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test -ldflags "$task_test_link_flags" -tags "$task_test_tags" \
      github.com/sagernet/sing-box/internal/xuispeed \
      github.com/sagernet/sing-box/transport/masque \
      github.com/sagernet/sing-box/protocol/masque \
      -run '^Test(Broker|InvalidBroker)' -count=1 -timeout=90s
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
python3 - "$task_root/deploy/hiddify/hiddify-core-target.json" "$task_output/target.json" "${target[3]}" "${SINGBOX_CRONET_LINKAGE:-purego}" <<'PYMANIFEST'
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

# CI audits the exact prepared source graph. Binary-only scans can conservatively
# report wildcard packages that are not linked, so keep source reachability too.
if [[ ${HIDDIFY_VULNCHECK:-0} == 1 ]]; then
  (
    cd -- "$task_source/core"
    CGO_ENABLED="$task_cgo" GOTOOLCHAIN="${target[5]}" go list -deps -tags "${target[3]}" ./cmd/main > "$task_source/packages.txt"
    if grep -Eq '^golang.org/x/crypto/openpgp(/|$)' "$task_source/packages.txt"; then
      echo 'Unmaintained OpenPGP package unexpectedly entered the kernel dependency graph' >&2
      exit 1
    fi
    echo 'Verified: deprecated OpenPGP packages are absent from the kernel import graph'
    CGO_ENABLED="$task_cgo" GOTOOLCHAIN="${target[5]}" GOMAXPROCS=4 GOMEMLIMIT=4GiB \
      go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -show verbose -tags "${target[3]}" ./cmd/main
  )
fi
