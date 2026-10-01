#!/usr/bin/env bash
set -euo pipefail

CONFIG_GLOB="/usr/local/x-ui/bin/mtproto/telemt-*.toml"
LEGACY_CONFIG="/etc/x-ui/telemt.toml"
FILTER_CHAIN="TELEMT_MEKO"
MARK_CHAIN="TELEMT_MEKO_MARK"
MARK="0x400"
U32_FILTER="32 & 0x000FFFFF = 0x0002FFFF && 40 & 0xFF000000 = 0x02000000 && 44 & 0xFFFF0000 = 0x01030000 && 48 & 0xFFFFFF00 = 0x01010800 && 60 & 0xFFFFFFFF = 0x04020000"
RATE="${TELEMT_MEKO_RATE:-54/minute}"
BURST="${TELEMT_MEKO_BURST:-1}"

log() { printf '%s\n' "[telemt-meko-fix] $*"; }

read_port() {
    local config="$1"
    awk '
        /^\[server\][[:space:]]*$/ { in_server=1; next }
        /^\[/ { in_server=0 }
        in_server && /^[[:space:]]*port[[:space:]]*=/ {
            line=$0
            sub(/^[^=]*=/, "", line)
            gsub(/[[:space:]]/, "", line)
            print line
            exit
        }
    ' "$config"
}

valid_port() {
    [[ "$1" =~ ^[0-9]+$ ]] && (( 10#$1 >= 1 && 10#$1 <= 65535 ))
}

collect_ports() {
    local -a found=()
    local p f

    if [[ -n "${TELEMT_PORTS:-}" ]]; then
        IFS=', ' read -r -a found <<< "$TELEMT_PORTS"
    else
        shopt -s nullglob
        for f in $CONFIG_GLOB; do
            p="$(read_port "$f" || true)"
            valid_port "$p" && found+=("$p")
        done
        shopt -u nullglob
        if (( ${#found[@]} == 0 )) && [[ -r "$LEGACY_CONFIG" ]]; then
            p="$(read_port "$LEGACY_CONFIG" || true)"
            valid_port "$p" && found+=("$p")
        fi
    fi

    printf '%s\n' "${found[@]}" | awk '/^[0-9]+$/ && $1>=1 && $1<=65535 { seen[$1]=1 } END { for (p in seen) print p }' | sort -n
}

ensure_u32() {
    if ! iptables -m u32 --help >/dev/null 2>&1; then
        modprobe xt_u32 >/dev/null 2>&1 || true
    fi
    iptables -m u32 --help >/dev/null 2>&1
}

get_ssh_port() {
    local p=""
    if command -v sshd >/dev/null 2>&1; then
        p="$(sshd -T 2>/dev/null | awk '$1=="port" {print $2; exit}' || true)"
    fi
    if ! valid_port "$p" && [[ -r /etc/ssh/sshd_config ]]; then
        p="$(awk 'tolower($1)=="port" && $2 ~ /^[0-9]+$/ {print $2; exit}' /etc/ssh/sshd_config || true)"
    fi
    valid_port "$p" || p=22
    printf '%s\n' "$p"
}

ensure_ssh_access() {
    local ssh_port
    ssh_port="$(get_ssh_port)"
    iptables -C INPUT -p tcp --dport "$ssh_port" -j ACCEPT 2>/dev/null || \
        iptables -I INPUT 1 -p tcp --dport "$ssh_port" -j ACCEPT
}

remove_jump_all() {
    local table="$1" parent="$2" child="$3"
    while iptables -t "$table" -C "$parent" -j "$child" 2>/dev/null; do
        iptables -t "$table" -D "$parent" -j "$child" 2>/dev/null || break
    done
}

ensure_chain() {
    local table="$1" chain="$2"
    iptables -t "$table" -N "$chain" 2>/dev/null || true
    iptables -t "$table" -F "$chain"
}

apply() {
    command -v iptables >/dev/null 2>&1 || { log "iptables is required"; exit 1; }
    ensure_u32 || { log "xt_u32 is not available; MEKO V3 cannot be enabled"; exit 1; }

    mapfile -t ports < <(collect_ports)
    (( ${#ports[@]} > 0 )) || { log "No active Telemt MTProto ports found"; exit 1; }

    ensure_ssh_access
    ensure_chain filter "$FILTER_CHAIN"
    ensure_chain mangle "$MARK_CHAIN"

    # Own the jumps explicitly. Recreate them so stale/duplicate rules from an
    # interrupted previous apply cannot change ordering or accumulate forever.
    remove_jump_all filter INPUT "$FILTER_CHAIN"
    remove_jump_all mangle PREROUTING "$MARK_CHAIN"
    iptables -t mangle -I PREROUTING 1 -j "$MARK_CHAIN"
    iptables -t filter -I INPUT 2 -j "$FILTER_CHAIN"

    local port
    for port in "${ports[@]}"; do
        valid_port "$port" || continue

        # MEKO V3 fingerprint: mark the characteristic iOS SYN and allow it
        # without the generic per-source SYN limiter.
        iptables -t mangle -A "$MARK_CHAIN" -p tcp --dport "$port" -m u32 --u32 "$U32_FILTER" \
            -j MARK --set-mark "$MARK"
        iptables -t filter -A "$FILTER_CHAIN" -p tcp --dport "$port" --syn -m mark --mark "$MARK" -j ACCEPT

        # Original MEKO V3 policy for all other clients: 54 SYN/min/IP, burst 1,
        # then an immediate TCP reset. Rate/burst can be overridden by the panel
        # service environment without modifying the script.
        iptables -t filter -A "$FILTER_CHAIN" -p tcp --dport "$port" --syn \
            -m hashlimit --hashlimit-name "telemt_meko_${port}" --hashlimit-mode srcip \
            --hashlimit-upto "$RATE" --hashlimit-burst "$BURST" \
            --hashlimit-htable-expire 60000 --hashlimit-htable-size 32768 -j ACCEPT
        iptables -t filter -A "$FILTER_CHAIN" -p tcp --dport "$port" --syn \
            -j REJECT --reject-with tcp-reset
    done

    log "MEKO V3 applied to Telemt MTProto ports: ${ports[*]} (rate=${RATE}, burst=${BURST})"
}

remove() {
    command -v iptables >/dev/null 2>&1 || return 0
    remove_jump_all filter INPUT "$FILTER_CHAIN"
    remove_jump_all mangle PREROUTING "$MARK_CHAIN"
    iptables -t filter -F "$FILTER_CHAIN" 2>/dev/null || true
    iptables -t filter -X "$FILTER_CHAIN" 2>/dev/null || true
    iptables -t mangle -F "$MARK_CHAIN" 2>/dev/null || true
    iptables -t mangle -X "$MARK_CHAIN" 2>/dev/null || true
    log "MEKO V3 rules removed"
}

status() {
    local installed=false
    if iptables -t filter -C INPUT -j "$FILTER_CHAIN" 2>/dev/null && \
       iptables -t mangle -C PREROUTING -j "$MARK_CHAIN" 2>/dev/null; then
        installed=true
    fi
    mapfile -t ports < <(collect_ports)
    printf 'installed=%s\n' "$installed"
    printf 'ports=%s\n' "$(IFS=,; echo "${ports[*]:-}")"
    printf 'rate=%s\n' "$RATE"
    printf 'burst=%s\n' "$BURST"
}

case "${1:-apply}" in
    apply) apply ;;
    remove) remove ;;
    status) status ;;
    *) echo "Usage: $0 {apply|remove|status}" >&2; exit 2 ;;
esac
