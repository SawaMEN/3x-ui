#!/usr/bin/env bash
# Shared pinned native engines for audit jobs. Do not use floating release URLs.
set -euo pipefail
arch="${1:?expected amd64 or arm64}"
case "$arch" in
  amd64)
    cronet_digest=23109c55b08829bb1a68682a61a64852b1bbf243753b153cbd8f6e4ea2c05294
    owner_digest=bab8ca4bb3bebe2c9e2555ee49d376d7f321265ff09b54af37591a8ff4f00c37
    xray_digest=0eec3ed4163c273d8c8bf821f7d219e257921cf584aa3fb970715e458e631978
    upstream_digest=699a1b510aa4396b2176d32d1181920399ebf1e267835da6f3b598d0f7c24968
    xray_asset=Xray-linux-64.zip ;;
  arm64)
    cronet_digest=3c1fcfcb56261a4b318ffb6d2227b9726782edb5a894e74db2b534406db62498
    owner_digest=5bf6d2f6bd99a01fb89dd99203980e7e0d6807bbf341bddef0c86b446912a77b
    xray_digest=d1304946c486d98e6b886e9b931eaaaebb23835b30e217638037b7f2721d4e9c
    upstream_digest=436441f76ee2f7fe21b5b2d426d3cb261a480455a704865789ad533e4273ec26
    xray_asset=Xray-linux-arm64-v8a.zip ;;
  *) echo "unsupported audit architecture: $arch" >&2; exit 2 ;;
esac
root="$RUNNER_TEMP/runtime-cores"
mkdir -p "$root/owner/x-ui/bin" "$root/xray" "$root/upstream" "$root/archives"
# stream1 was built from the locked commits and checked-in overlays, with
# native stream regression tests, by protocol audit 41421583366. Reuse exactly
# those bytes; the regular release build still rebuilds and tests the sources.
gh run download 41421583366 --repo SawaMEN/3x-ui --name protocol-owner-core-${arch} --dir "$root/owner/x-ui/bin"
echo "${owner_digest} $root/owner/x-ui/bin/hiddify-core-linux-${arch}" | sha256sum -c -
chmod 0700 "$root/owner/x-ui/bin/hiddify-core-linux-${arch}"
# The audited owner profile uses purego, so Naive needs its matching Cronet
# beside the executable. Pin both the source commit and the library bytes.
cronet_revision=0d28acc44093df24b2526dea3d6ffefd6b0a54f0
git init -q "$root/cronet-source"
git -C "$root/cronet-source" remote add origin https://github.com/sagernet/cronet-go.git
git -C "$root/cronet-source" sparse-checkout set --no-cone "/lib/linux_${arch}/libcronet.so"
git -C "$root/cronet-source" fetch --depth=1 --filter=blob:none origin "$cronet_revision"
git -C "$root/cronet-source" checkout -q --detach FETCH_HEAD
cp "$root/cronet-source/lib/linux_${arch}/libcronet.so" "$root/owner/x-ui/bin/libcronet.so"
echo "${cronet_digest} $root/owner/x-ui/bin/libcronet.so" | sha256sum -c -
gh release download v26.10.21 --repo XTLS/Xray-core --pattern ${xray_asset} --dir "$root/archives"
echo "${xray_digest} $root/archives/${xray_asset}" | sha256sum -c -
unzip -q "$root/archives/${xray_asset}" -d "$root/xray"
gh release download v1.14.4 --repo SagerNet/sing-box --pattern sing-box-1.14.4-linux-${arch}.tar.gz --dir "$root/archives"
echo "${upstream_digest} $root/archives/sing-box-1.14.4-linux-${arch}.tar.gz" | sha256sum -c -
tar -xzf "$root/archives/sing-box-1.14.4-linux-${arch}.tar.gz" --strip-components=1 -C "$root/upstream"
echo "XUI_STAGE4_CORE_DIR=$root" >> "$GITHUB_ENV"
sha256sum "$root/owner/x-ui/bin/hiddify-core-linux-${arch}" "$root/xray/xray" "$root/upstream/sing-box" "$root/owner/x-ui/bin/libcronet.so" | tee native-binaries.sha256
