#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
SOURCE_DIR=${1:?usage: build.sh SOURCE_DIR OUTPUT_DIR [TARGET]}
OUTPUT_DIR=${2:?usage: build.sh SOURCE_DIR OUTPUT_DIR [TARGET]}
TARGET=${3:-x86_64-unknown-linux-musl}
readarray -t source_info < <(python3 - "$ROOT/source.json" <<'PY'
import json,sys
source=json.load(open(sys.argv[1]))
for key in ('repository','commit','version'):
    print(source[key])
PY
)
REPOSITORY=${source_info[0]}
COMMIT=${source_info[1]}
VERSION=${source_info[2]}
mkdir -p "$SOURCE_DIR" "$OUTPUT_DIR"
SOURCE_DIR=$(realpath "$SOURCE_DIR")
OUTPUT_DIR=$(realpath "$OUTPUT_DIR")
if [[ ! -d "$SOURCE_DIR/.git" ]]; then
  git -C "$SOURCE_DIR" init -q
  git -C "$SOURCE_DIR" fetch --depth 1 "$REPOSITORY" "$COMMIT"
  git -C "$SOURCE_DIR" checkout --detach FETCH_HEAD
fi
[[ $(git -C "$SOURCE_DIR" rev-parse HEAD) == "$COMMIT" ]] || { echo 'Wrong Telemt source revision' >&2; exit 1; }
git -C "$SOURCE_DIR" diff --quiet && git -C "$SOURCE_DIR" diff --cached --quiet || { echo 'Telemt build requires a clean source checkout' >&2; exit 1; }
git -C "$SOURCE_DIR" apply --check "$ROOT/3x-ui.patch"
git -C "$SOURCE_DIR" apply "$ROOT/3x-ui.patch"
python3 - "$SOURCE_DIR" "$VERSION" <<'PY'
from pathlib import Path
import re,sys
root=Path(sys.argv[1]);version=sys.argv[2]
p=root/'Cargo.toml';s=p.read_text();s=re.sub(r'(?m)^version = "[^"]+"',f'version = "{version}"',s,count=1);p.write_text(s)
p=root/'Cargo.lock';s=p.read_text();s=re.sub(r'(\[\[package\]\]\nname = "telemt"\nversion = ")[^"]+',lambda m:m[1]+version,s,count=1);p.write_text(s)
PY
cd "$SOURCE_DIR"
cargo test --locked -- --test-threads=4
if [[ ${XUI_TELEMT_TEST_ONLY:-0} == 1 ]]; then exit 0; fi
cargo build --release --locked --target "$TARGET"
install -m 0755 "${CARGO_TARGET_DIR:-$SOURCE_DIR/target}/$TARGET/release/telemt" "$OUTPUT_DIR/telemt"
printf '%s\n' "$VERSION" > "$OUTPUT_DIR/telemt.version"
case "$TARGET" in
 x86_64-*) PLATFORM=amd64 ;;
 aarch64-*) PLATFORM=arm64 ;;
 *) echo "Unsupported target: $TARGET" >&2; exit 1 ;;
esac
cd "$OUTPUT_DIR"
tar -czf "telemt-linux-${PLATFORM}.tar.gz" telemt telemt.version
sha256sum "telemt-linux-${PLATFORM}.tar.gz" > "telemt-linux-${PLATFORM}.tar.gz.sha256"
