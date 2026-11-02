#!/usr/bin/env bash
# Shared pinned native engines for audit jobs. Do not use floating release URLs.
set -euo pipefail
arch="${1:?expected amd64 or arm64}"
case "$arch" in
  amd64)
    owner_digest=a0a1ded3f54ac6cf9ce305c49836b00194e7d73449ad8a96d68fdf82a24a0a94
    xray_digest=0eec3ed4163c273d8c8bf821f7d219e257921cf584aa3fb970715e458e631978
    upstream_digest=699a1b510aa4396b2176d32d1181920399ebf1e267835da6f3b598d0f7c24968
    xray_asset=Xray-linux-64.zip ;;
  arm64)
    owner_digest=e58e2fc844eac4b3ab87a875faebc5bdfa6bef848d183415449959f3dc3896b4
    xray_digest=d1304946c486d98e6b886e9b931eaaaebb23835b30e217638037b7f2721d4e9c
    upstream_digest=436441f76ee2f7fe21b5b2d426d3cb261a480455a704865789ad533e4273ec26
    xray_asset=Xray-linux-arm64-v8a.zip ;;
  *) echo "unsupported audit architecture: $arch" >&2; exit 2 ;;
esac
root="$RUNNER_TEMP/runtime-cores"
mkdir -p "$root/owner" "$root/xray" "$root/upstream" "$root/archives"
gh run download 41326858660 --repo SawaMEN/3x-ui --name x-ui-linux-${arch} --dir "$root/archives"
echo "${owner_digest} $root/archives/x-ui-linux-${arch}.tar.gz" | sha256sum -c -
tar -xzf "$root/archives/x-ui-linux-${arch}.tar.gz" -C "$root/owner" x-ui/bin/hiddify-core-linux-${arch}
gh release download v26.10.21 --repo XTLS/Xray-core --pattern ${xray_asset} --dir "$root/archives"
echo "${xray_digest} $root/archives/${xray_asset}" | sha256sum -c -
unzip -q "$root/archives/${xray_asset}" -d "$root/xray"
gh release download v1.14.4 --repo SagerNet/sing-box --pattern sing-box-1.14.4-linux-${arch}.tar.gz --dir "$root/archives"
echo "${upstream_digest} $root/archives/sing-box-1.14.4-linux-${arch}.tar.gz" | sha256sum -c -
tar -xzf "$root/archives/sing-box-1.14.4-linux-${arch}.tar.gz" --strip-components=1 -C "$root/upstream"
echo "XUI_STAGE4_CORE_DIR=$root" >> "$GITHUB_ENV"
sha256sum "$root/owner/x-ui/bin/hiddify-core-linux-${arch}" "$root/xray/xray" "$root/upstream/sing-box" | tee native-binaries.sha256
