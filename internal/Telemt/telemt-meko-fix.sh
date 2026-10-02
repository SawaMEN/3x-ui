#!/usr/bin/env bash
set -euo pipefail

CONFIG_GLOB="/usr/local/x-ui/bin/mtproto/telemt-*.toml"
LEGACY_CONFIG="/etc/x-ui/telemt.toml"
FILTER_CHAIN="TELEMT_MEKO"
MARK_CHAIN="TELEMT_MEKO_MARK"
MARK="0x400"
U32_FILTER="32 & 0x000FFFFF = 0x0002FFFF && 40 & 0xFF000000 = 0x02000000 && 44 & 0xFFFF0000 = 0x01030000 && 48 & 0xFFFFFF00 = 0x01010800 && 60 & 0xFFFFFFFF = 0x04020000"
ENABLED="${TELEMT_MEKO_ENABLED:-0}"
RATE="${TELEMT_MEKO_RATE:-54/minute}"
BURST="${TELEMT_MEKO_BURST:-1}"

log() { printf '%s\n' "[telemt-meko-fix] $*"; }

enabled() {
    case "${ENABLED,,}" in
        1|true|yes|on) return 0 ;;
        *) return 1 ;;
    esac
}

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

        if [[ -r "$LEGACY_CONFIG" ]] && command -v systemctl >/dev/null 2>&1 && \
           systemctl is-active --quiet telemt.service 2>/dev/null; then
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

apply() {
    if ! enabled; then
        remove
        log "MEKO V3 is disabled by panel settings"
        return 0
    fi

    command -v iptables >/dev/null 2>&1 || { log "iptables is required"; exit 1; }
    ensure_u32 || { log "xt_u32 is not available; MEKO V3 cannot be enabled"; exit 1; }

    mapfile -t ports < <(collect_ports)
    if (( ${#ports[@]} == 0 )); then
        remove
        log "No active Telemt MTProto ports found; rules left clean"
        return 0
    fi

    ensure_ssh_access
    ensure_chain filter "$FILTER_CHAIN"
    ensure_chain mangle "$MARK_CHAIN"

    remove_jump_all filter INPUT "$FILTER_CHAIN"
    remove_jump_all mangle PREROUTING "$MARK_CHAIN"
    iptables -t mangle -I PREROUTING 1 -j "$MARK_CHAIN"
    iptables -t filter -I INPUT 2 -j "$FILTER_CHAIN"

    local port
    for port in "${ports[@]}"; do
        valid_port "$port" || continue

        iptables -t mangle -A "$MARK_CHAIN" -p tcp --dport "$port" -m u32 --u32 "$U32_FILTER" \
            -j MARK --set-mark "$MARK"
        iptables -t filter -A "$FILTER_CHAIN" -p tcp --dport "$port" --syn -m mark --mark "$MARK" -j ACCEPT

        iptables -t filter -A "$FILTER_CHAIN" -p tcp --dport "$port" --syn \
            -m hashlimit --hashlimit-name "telemt_meko_${port}" --hashlimit-mode srcip \
            --hashlimit-upto "$RATE" --hashlimit-burst "$BURST" \
            --hashlimit-htable-expire 60000 --hashlimit-htable-size 32768 -j ACCEPT
        iptables -t filter -A "$FILTER_CHAIN" -p tcp --dport "$port" --syn \
            -j REJECT --reject-with tcp-reset
    done

    log "MEKO V3 applied to Telemt MTProto ports: ${ports[*]} (rate=${RATE}, burst=${BURST})"
}

status() {
    local installed=false
    local -a applied_ports=()
    if command -v iptables >/dev/null 2>&1; then
        if iptables -t filter -C INPUT -j "$FILTER_CHAIN" 2>/dev/null && \
           iptables -t mangle -C PREROUTING -j "$MARK_CHAIN" 2>/dev/null; then
            installed=true
        fi
        if iptables -t filter -S "$FILTER_CHAIN" >/dev/null 2>&1; then
            mapfile -t applied_ports < <(
                iptables -t filter -S "$FILTER_CHAIN" 2>/dev/null \
                    | awk '{ for (i=1; i<=NF; i++) if ($i=="--dport") print $(i+1) }' \
                    | awk '/^[0-9]+$/ { seen[$1]=1 } END { for (p in seen) print p }' \
                    | sort -n
            )
        fi
    fi
    mapfile -t ports < <(collect_ports)
    printf 'enabled=%s\n' "$(enabled && echo true || echo false)"
    printf 'installed=%s\n' "$installed"
    printf 'ports=%s\n' "$(IFS=,; echo "${ports[*]:-}")"
    printf 'applied_ports=%s\n' "$(IFS=,; echo "${applied_ports[*]:-}")"
    printf 'rate=%s\n' "$RATE"
    printf 'burst=%s\n' "$BURST"
}

case "${1:-apply}" in
    apply) apply ;;
    remove) remove ;;
    status) status ;;
    *) echo "Usage: $0 {apply|remove|status}" >&2; exit 2 ;;
esac
