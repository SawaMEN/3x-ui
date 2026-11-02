#!/usr/bin/env bash
# Shared pinned native engines for audit jobs. Do not use floating release URLs.
set -euo pipefail
arch="${1:?expected amd64 or arm64}"
case "$arch" in
  amd64)
    xray_digest=0eec3ed4163c273d8c8bf821f7d219e257921cf584aa3fb970715e458e631978
    upstream_digest=699a1b510aa4396b2176d32d1181920399ebf1e267835da6f3b598d0f7c24968
    xray_asset=Xray-linux-64.zip ;;
  arm64)
    xray_digest=d1304946c486d98e6b886e9b931eaaaebb23835b30e217638037b7f2721d4e9c
    upstream_digest=436441f76ee2f7fe21b5b2d426d3cb261a480455a704865789ad533e4273ec26
    xray_asset=Xray-linux-arm64-v8a.zip ;;
  *) echo "unsupported audit architecture: $arch" >&2; exit 2 ;;
esac
root="$RUNNER_TEMP/runtime-cores"
mkdir -p "$root/owner" "$root/xray" "$root/upstream" "$root/archives"
# Build the owner engine from the manifest's pinned commits and checked-in
# overlays. This also runs the transport regression tests on the native runner.
bash tools/build-hiddify-core.sh "$root/owner/x-ui/bin"
gh release download v26.10.21 --repo XTLS/Xray-core --pattern ${xray_asset} --dir "$root/archives"
echo "${xray_digest} $root/archives/${xray_asset}" | sha256sum -c -
unzip -q "$root/archives/${xray_asset}" -d "$root/xray"
gh release download v1.14.4 --repo SagerNet/sing-box --pattern sing-box-1.14.4-linux-${arch}.tar.gz --dir "$root/archives"
echo "${upstream_digest} $root/archives/sing-box-1.14.4-linux-${arch}.tar.gz" | sha256sum -c -
tar -xzf "$root/archives/sing-box-1.14.4-linux-${arch}.tar.gz" --strip-components=1 -C "$root/upstream"
echo "XUI_STAGE4_CORE_DIR=$root" >> "$GITHUB_ENV"
sha256sum "$root/owner/x-ui/bin/hiddify-core-linux-${arch}" "$root/xray/xray" "$root/upstream/sing-box" | tee native-binaries.sha256
