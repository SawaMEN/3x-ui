#!/bin/bash
set -euo pipefail

BIN="/usr/local/x-ui/bin/telemt"
VERSION_FILE="/etc/x-ui/telemt.version"
TMP_DIR=""
REPO="telemt/telemt"
cleanup() { [[ -z "$TMP_DIR" ]] && return; rm -rf "$TMP_DIR"; }
trap cleanup EXIT
log() { echo "[telemt-update] $*"; }

arch_name() {
  case "$(uname -m)" in
    x86_64|amd64) echo "x86_64" ;;
    aarch64|arm64) echo "aarch64" ;;
    *) echo "unsupported" ;;
  esac
}

get_latest_tag() {
  # Only the official published stable release; GitHub excludes prereleases.
  curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 \
    --connect-timeout 10 --max-time 60 \
    "https://api.github.com/repos/${REPO}/releases/latest" |
    sed -nE 's/^[[:space:]]*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p'
}

binary_version() {
  "$1" --version 2>/dev/null |
    sed -nE 's/^[^0-9]*v?([0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?).*$/\1/p' |
    sed -n '1p'
}

current_version() {
  if [[ -x "$BIN" ]]; then
    binary_version "$BIN"
    return
  fi
  if [[ -s "$VERSION_FILE" ]]; then tr -d '[:space:]' < "$VERSION_FILE"; fi
}

# Do not silently replace a newer installed build with the older latest
# stable release. A same-version legacy +3xui fork is still migrated to the
# official unmodified binary.
version_is_newer() {
  local installed="${1%%[+-]*}" latest="${2%%[+-]*}"
  installed="${installed#v}"; latest="${latest#v}"
  [[ "$installed" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ && "$latest" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || return 1
  local ia ib ic la lb lc
  IFS=. read -r ia ib ic <<< "$installed"
  IFS=. read -r la lb lc <<< "$latest"
  (( 10#$ia > 10#$la )) && return 0
  (( 10#$ia < 10#$la )) && return 1
  (( 10#$ib > 10#$lb )) && return 0
  (( 10#$ib < 10#$lb )) && return 1
  (( 10#$ic > 10#$lc ))
}

download_latest() {
  local arch="$1" tag="$2" version="$3"
  local asset="telemt-${arch}-linux-musl.tar.gz"
  local base="https://github.com/${REPO}/releases/download/${tag}"
  [[ -n "$TMP_DIR" ]] || TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/telemt-update.XXXXXX")
  mkdir -p "$TMP_DIR"
  curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --connect-timeout 10 --max-time 180 -o "$TMP_DIR/$asset" "$base/$asset"
  curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --connect-timeout 10 --max-time 60 -o "$TMP_DIR/$asset.sha256" "$base/$asset.sha256"
  ( cd "$TMP_DIR"; sha256sum -c "$asset.sha256" )
  [[ "$(tar -tzf "$TMP_DIR/$asset")" == "telemt" ]] || { echo "Unexpected official Telemt archive layout" >&2; return 1; }
  tar -xzf "$TMP_DIR/$asset" -C "$TMP_DIR" --no-same-owner --no-same-permissions telemt
  chmod 0755 "$TMP_DIR/telemt"
  [[ "$(binary_version "$TMP_DIR/telemt")" == "$version" ]] || { echo "Official Telemt binary version mismatch" >&2; return 1; }
}

install_latest() {
  local version="$1"
  install -d -m 0755 -o root -g root "$(dirname "$BIN")"
  install -m 0755 -o root -g root "$TMP_DIR/telemt" "${BIN}.new"
  mv -f "${BIN}.new" "$BIN"
  printf '%s\n' "$version" > "$VERSION_FILE"
  chown root:root "$VERSION_FILE"
  chmod 0644 "$VERSION_FILE"
}

recycle_inbound_sidecars() {
  # Panel-managed MTProto sidecars respawn using the newly installed binary.
  local pattern="${BIN} run /usr/local/x-ui/bin/mtproto/telemt-"
  local pids=""
  if command -v pgrep >/dev/null 2>&1; then
    pids="$(pgrep -f "$pattern" 2>/dev/null || true)"
  else
    pids="$(ps -eo pid=,args= 2>/dev/null | awk -v p="$pattern" 'index($0,p) {print $1}')"
  fi
  [[ -n "$pids" ]] || return 0
  while read -r pid; do
    [[ "$pid" =~ ^[0-9]+$ ]] || continue
    kill -TERM "$pid" 2>/dev/null || true
  done <<< "$pids"
}

main() {
  [[ "$EUID" -eq 0 ]] || { echo "must run as root" >&2; exit 1; }
  if [[ "${1:-}" != "--check" ]]; then
    command -v flock >/dev/null 2>&1 || { echo "flock (util-linux) is required" >&2; exit 1; }
    exec 9>/run/lock/x-ui-telemt-update.lock
    flock -w 60 9 || { echo "another Telemt update is in progress" >&2; exit 1; }
  fi
  local arch tag version current was_active=0
  arch="$(arch_name)"
  [[ "$arch" != "unsupported" ]] || { echo "unsupported CPU architecture" >&2; exit 1; }
  tag="$(get_latest_tag)"
  [[ "$tag" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "invalid official Telemt tag: $tag" >&2; exit 1; }
  version="${tag#v}"
  current="$(current_version || true)"
  if [[ "${1:-}" == "--check" ]]; then
    echo "current=${current:-unknown}"
    echo "latest=$version"
    if [[ "${current#v}" == "$version" ]] || version_is_newer "$current" "$version"; then
      exit 0
    fi
    exit 1
  fi
  if [[ -x "$BIN" ]] && { [[ "${current#v}" == "$version" ]] || version_is_newer "$current" "$version"; }; then
    log "Telemt ${current#v} is already installed (latest stable: $version)."
    exit 0
  fi
  log "updating Telemt ${current:-unknown} -> official $version"
  download_latest "$arch" "$tag" "$version"
  # All validations must complete before stopping a live service.
  [[ ! -e "$BIN" ]] || cp -p "$BIN" "$TMP_DIR/previous-telemt"
  [[ ! -e "$VERSION_FILE" ]] || cp -p "$VERSION_FILE" "$TMP_DIR/previous-version"
  if systemctl is-active --quiet telemt.service 2>/dev/null; then
    was_active=1
    systemctl stop telemt.service
  fi
  if ! install_latest "$version" || { (( was_active )) && ! systemctl start telemt.service; }; then
    log "Installation or startup failed; rolling back"
    rm -f "${BIN}.new"
    if [[ -f "$TMP_DIR/previous-telemt" ]]; then
      cp -p "$TMP_DIR/previous-telemt" "${BIN}.new"
      mv -f "${BIN}.new" "$BIN"
    else
      rm -f "$BIN"
    fi
    if [[ -f "$TMP_DIR/previous-version" ]]; then
      cp -p "$TMP_DIR/previous-version" "$VERSION_FILE"
    else
      rm -f "$VERSION_FILE"
    fi
    if (( was_active )); then systemctl start telemt.service || true; fi
    return 1
  fi
  recycle_inbound_sidecars
  log "Official Telemt $version installed."
}
main "$@"
