#!/usr/bin/env bash
set -euo pipefail
# Apply only to this exact upstream source. Never patch an arbitrary new tag.
UPSTREAM=113cdc89559b9d2698e1477b3d0af89e95c0b7cf
VERSION=1.3.1-xui.1
src=${1:?usage: build.sh SOURCE_DIR OUTPUT_DIR}
out=${2:?usage: build.sh SOURCE_DIR OUTPUT_DIR}
[[ "$(git -C "$src" rev-parse HEAD)" == "$UPSTREAM" ]]
mkdir -p "$out"
out=$(cd "$out" && pwd)
python3 - "$src" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]) / "lib/src/core.rs"
s = p.read_text()
anchor = """        let has_tcp_based_codec =
            settings.listen_protocols.http1.is_some() || settings.listen_protocols.http2.is_some();

        let tcp_listener = {"""
replacement = """        let has_tcp_based_codec =
            settings.listen_protocols.http1.is_some() || settings.listen_protocols.http2.is_some();

        // 3x-ui: an explicitly QUIC-only endpoint must not reserve TCP.
        // Combined and TCP-only endpoints retain the upstream behavior.
        if !has_tcp_based_codec {
            return Ok(());
        }

        let tcp_listener = {"""
if s.count(anchor) != 1:
    raise SystemExit("upstream TCP listener anchor changed; review the patch")
p.write_text(s.replace(anchor, replacement, 1))

# The private metrics listener also uses Http1Codec, even when the public
# HTTP/1 protocol is disabled. Preserve configured buffer sizes, but use
# the upstream default for that internal HTTP/1 codec in QUIC-only mode.
p = pathlib.Path(sys.argv[1]) / "lib/src/http1_codec.rs"
s = p.read_text()
anchor = """                .http1
                .as_ref()
                .unwrap()
                .upload_buffer_size,"""
replacement = """                .http1
                .as_ref()
                .map(|settings| settings.upload_buffer_size)
                .unwrap_or_else(crate::settings::Http1Settings::default_upload_buffer_size),"""
if s.count(anchor) != 1:
    raise SystemExit("upstream metrics codec anchor changed; review the patch")
p.write_text(s.replace(anchor, replacement, 1))
PY
cd "$src"
export TRUSTTUNNEL_VERSION="$VERSION"
export CARGO_PROFILE_RELEASE_DEBUG=0
case "$(uname -m)" in
  x86_64) arch=x86_64 ;;
  aarch64) arch=aarch64 ;;
  *) echo "Unsupported native build architecture" >&2; exit 1 ;;
esac
# Build on Ubuntu 22.04 for a conservative glibc baseline. CI runs the
# resulting native executable on each matching architecture before publishing.
cargo build --locked --release -p trusttunnel_endpoint
target=target/release/trusttunnel_endpoint
"$target" --version | grep -F "$VERSION"
name="trusttunnel-v${VERSION}-linux-${arch}"
mkdir -p "$out/$name"
install -m 0755 "$target" "$out/$name/trusttunnel_endpoint"
cp LICENSE "$out/$name/LICENSE"
printf 'Upstream: https://github.com/TrustTunnel/TrustTunnel\nCommit: %s\nModifications: do not bind TCP without HTTP/1 or HTTP/2; retain internal HTTP/1 metrics with upstream buffer defaults.\n' "$UPSTREAM" > "$out/$name/3X-UI-MODIFICATIONS.txt"
[[ ! -f NOTICE ]] || cp NOTICE "$out/$name/NOTICE"
tar -C "$out" -czf "$out/$name.tar.gz" "$name"
(cd "$out" && sha256sum "$name.tar.gz" > "$name.tar.gz.sha256")
