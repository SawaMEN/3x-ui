#!/usr/bin/env bash
# Install the latest *compatible* stable core independently of panel releases.
# The panel and its database remain untouched. On failure restore the prior core.
set -Eeuo pipefail

if (( EUID != 0 )); then
  echo "Hiddify updater requires root" >&2
  exit 1
fi
case "${1:-}" in
  ""|--check) ;;
  *) echo "Usage: hiddify-core-update.sh [--check]" >&2; exit 2 ;;
esac
if [[ "${XUI_AUTO_CORE_UPDATE:-true}" == false && "${1:-}" != --check ]]; then
  echo "Automatic Hiddify updates disabled"
  exit 0
fi
for command_name in python3 flock systemctl mktemp sha256sum; do
  command -v "$command_name" >/dev/null || { echo "Missing $command_name" >&2; exit 1; }
done
root="${XUI_MAIN_FOLDER:-/usr/local/x-ui}"
[[ "$root" == /* && "$root" != / && -d "$root" && ! -L "$root" ]] || {
  echo "Invalid x-ui installation root" >&2
  exit 1
}
bin="${XUI_BIN_FOLDER:-$root/bin}"
[[ "$bin" == /* ]] || bin="$root/$bin"
[[ -d "$bin" && ! -L "$bin" && -x "$root/x-ui" ]] || {
  echo "Missing panel or core binary directory" >&2
  exit 1
}
helper="$root/fetch-latest-kernel.py"
[[ -f "$helper" && ! -L "$helper" ]] || { echo "Release downloader is missing" >&2; exit 1; }
case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "This updater supports Linux AMD64 and ARM64 only" >&2; exit 1 ;;
esac
binary="hiddify-core-linux-$arch"
assets=("$binary" "$binary.sha256" libcronet.so libcronet.so.sha256 target.json)
# Serialize with the panel's install/update and migration transactions.
parent="$(dirname "$root")"
exec 9>"$parent/.x-ui-upgrade.lock"
if ! flock -n 9; then
  echo "A panel migration or core update is already running; skipping"
  exit 0
fi

tag=$(python3 "$helper" --latest-tag)
[[ -n "$tag" ]] || { echo "No complete stable core release" >&2; exit 1; }
current_tag=""
if [[ -f "$bin/target.json" ]]; then
  current_tag=$(python3 - "$bin/target.json" <<'PY'
import json, sys
try:
    data = json.load(open(sys.argv[1]))
    print(data["target"]["version"])
except (ValueError, OSError, KeyError, TypeError):
    sys.exit("Installed core manifest is damaged; refusing automatic replacement")
PY
)
fi
echo "Installed core: ${current_tag:-unknown}; newest stable core: $tag"
if [[ "$current_tag" == "$tag" ]]; then
  exit 0
fi
if [[ "${1:-}" == --check ]]; then
  echo "Update available: $tag"
  exit 0
fi

stage=$(mktemp -d "$root/.core-stage.XXXXXXXX")
backup=$(mktemp -d "$root/.core-backup.XXXXXXXX")
swapping=0
was_active=0
committed=0
cleanup() {
  status=$?
  trap - EXIT INT TERM
  local restore_failed=0
  if (( swapping && ! committed )); then
    echo "New core failed health checks: restoring prior core" >&2
    systemctl stop x-ui >/dev/null 2>&1 || true
    for asset in "${assets[@]}"; do
      rm -f -- "$bin/$asset" || restore_failed=1
      if [[ -f "$backup/$asset" ]]; then
        cp -p -- "$backup/$asset" "$bin/$asset" || { echo "CRITICAL: failed to restore $asset" >&2; restore_failed=1; }
      fi
    done
    if (( was_active )); then
      systemctl start x-ui || { echo "CRITICAL: prior panel failed to restart" >&2; restore_failed=1; }
    fi
  fi
  for asset in "${assets[@]}"; do rm -f -- "$bin/.core-update-$asset.$"; done
  rm -rf -- "$stage"
  if (( restore_failed )); then
    echo "CRITICAL: recovery snapshot preserved at $backup" >&2
    exit 1
  fi
  rm -rf -- "$backup"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Resolve by exact tag; the download validates archive, binary, Cronet digests,
# ELF architecture, source/release SHA identity and required feature tags.
python3 "$helper" --tag "$tag" --arch "$arch" --output "$stage"
# Never silently change the panel/kernel ABI or protection profile. A future
# incompatible release is retried after the panel's own compatibility update.
python3 - "$bin/target.json" "$stage/target.json" <<'PY'
import json, pathlib, re, sys
old_path, next_path = map(pathlib.Path, sys.argv[1:])
if not old_path.is_file():
    sys.exit("Cannot establish old core's compatibility profile")
old, new = json.loads(old_path.read_text()), json.loads(next_path.read_text())
previous = old["target"]["version"]
current = new["target"]["version"]
pattern = r"^sawamen-[a-f0-9]{12}-[a-f0-9]{12}(-mask[0-9]+(?:-[a-z0-9]+)*)$"
a, b = re.fullmatch(pattern, previous), re.fullmatch(pattern, current)
if a is None or b is None or a.group(1) != b.group(1):
    sys.exit("Latest kernel changes the reviewed feature profile; leaving old kernel in use")
required = set(old["panel_build"]["tags"])
provided = set(new["panel_build"]["tags"])
if not required.issubset(provided):
    sys.exit("Latest kernel removed features used by this panel")
if (old["runtime_contract"]["stats_grpc_method"] != new["runtime_contract"]["stats_grpc_method"]
    or old["panel_build"]["runtime_dependencies"] != new["panel_build"]["runtime_dependencies"]):
    sys.exit("Latest kernel changes the panel runtime ABI")
PY

# Validate the installed config with the new binary before touching any live
# files or listeners. Keep cwd in bin for relative geo and certificate paths.
if [[ -s "$bin/hiddify-core.json" ]]; then
  (cd "$bin" && "$stage/$binary" check -c "$bin/hiddify-core.json")
fi
for asset in "${assets[@]}"; do
  [[ -f "$stage/$asset" && ! -L "$stage/$asset" ]] || exit 1
  if [[ -e "$bin/$asset" ]]; then
    [[ -f "$bin/$asset" && ! -L "$bin/$asset" ]] || exit 1
    cp -p -- "$bin/$asset" "$backup/$asset"
  fi
done
if systemctl is-active --quiet x-ui; then was_active=1; fi
swapping=1
if (( was_active )); then systemctl stop x-ui; fi
for asset in "${assets[@]}"; do
  # Stage each new member in the destination filesystem; each rename is atomic.
  cp -p -- "$stage/$asset" "$bin/.core-update-$asset.$$"
  mv -f -- "$bin/.core-update-$asset.$$" "$bin/$asset"
done
chmod 0755 "$bin/$binary"
(cd "$bin" && "./$binary" version | grep -F "hiddify-core version $tag ")
if (( was_active )); then
  systemctl start x-ui
  sleep 2
  systemctl is-active --quiet x-ui
  "$root/x-ui" core-migrate --verify-running
fi
committed=1
echo "Updated Hiddify core to $tag; prior panel and data preserved"
