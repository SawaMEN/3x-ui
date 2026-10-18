#!/usr/bin/env bash
set -euo pipefail

# The panel, systemd and the sidecar supervisor can invoke this helper at once.
# Serialize readers and writers across processes, including loading settings.
command -v flock >/dev/null 2>&1 || { echo "flock (util-linux) is required" >&2; exit 1; }
exec 9>/run/lock/x-ui-telemt-meko.lock
flock -w 8 9 || { echo "MEKO rule update is already in progress" >&2; exit 1; }

# Wait for the xtables lock held by other firewall tools as well.
iptables() { command iptables -w 2 "$@"; }

CONFIG_GLOB="/usr/local/x-ui/bin/mtproto/telemt-*.toml"
LEGACY_CONFIG="/etc/x-ui/telemt.toml"
STATE_FILE="/etc/x-ui/telemt-meko-fix.env"
FILTER_CHAIN="TELEMT_MEKO"
MARK_CHAIN="TELEMT_MEKO_MARK"
MARK="0x400"
U32_FILTER="32 & 0x000FFFFF = 0x0002FFFF && 40 & 0xFFFF0000 = 0x02040000 && 44 & 0xFFFF0000 = 0x01030000 && 48 & 0xFFFFFF00 = 0x01010800 && 60 & 0xFFFFFFFF = 0x04020000"

# Load panel-managed settings for direct/manual invocations too. Explicit
# environment values still win, which lets the backend preview/apply settings
# atomically before the systemd unit is restarted.
file_enabled=""
file_rate=""
file_burst=""
if [[ -r "$STATE_FILE" ]]; then
    while IFS='=' read -r key value; do
        case "${key//[[:space:]]/}" in
            TELEMT_MEKO_ENABLED) file_enabled="${value//[[:space:]]/}" ;;
            TELEMT_MEKO_RATE) file_rate="${value//[[:space:]]/}" ;;
            TELEMT_MEKO_BURST) file_burst="${value//[[:space:]]/}" ;;
        esac
    done < "$STATE_FILE"
fi
ENABLED="${TELEMT_MEKO_ENABLED:-${file_enabled:-1}}"
RATE="${TELEMT_MEKO_RATE:-${file_rate:-54/minute}}"
BURST="${TELEMT_MEKO_BURST:-${file_burst:-1}}"

log() { printf '%s\n' "[telemt-meko-fix] $*"; }

enabled() {
    case "${ENABLED,,}" in
        1|true|yes|on) return 0 ;;
        *) return 1 ;;
    esac
}

normalize_rate() {
    local value="$1" number
    case "$value" in
        */minute) number="${value%/minute}" ;;
        */min) number="${value%/min}" ;;
        *) number="$value" ;;
    esac
    [[ "$number" =~ ^[0-9]+$ ]] || return 1
    (( 10#$number >= 1 && 10#$number <= 60000 )) || return 1
    printf '%d/minute\n' "$((10#$number))"
}

normalize_burst() {
    local value="$1"
    [[ "$value" =~ ^[0-9]+$ ]] || return 1
    (( 10#$value >= 1 && 10#$value <= 1000 )) || return 1
    printf '%d\n' "$((10#$value))"
}

normalize_limits() {
    local raw_rate="$RATE" raw_burst="$BURST"
    if ! RATE="$(normalize_rate "$raw_rate")"; then
        log "Invalid TELEMT_MEKO_RATE: $raw_rate"
        return 2
    fi
    if ! BURST="$(normalize_burst "$raw_burst")"; then
        log "Invalid TELEMT_MEKO_BURST: $raw_burst"
        return 2
    fi
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

# Return sorted port/rate/burst triples. The matching sidecar TOML is the
# sole authority: no generic TCP listener, WEB backend or old Telemt config is
# ever selected merely because it listens on the same server.
collect_port_policies() {
    local p f rate burst
    if [[ -n "${TELEMT_PORTS:-}" ]]; then
        local -a overridden=()
        IFS=', ' read -r -a overridden <<< "$TELEMT_PORTS"
        for p in "${overridden[@]}"; do
            valid_port "$p" || continue
            printf '%s %s %s\n' "$((10#$p))" "$RATE" "$BURST"
        done
    else
        shopt -s nullglob
        for f in $CONFIG_GLOB; do
            [[ "$(head -n 1 "$f" 2>/dev/null)" == "# x-ui-meko-fix=1" ]] || continue
            p="$(read_port "$f" || true)"
            valid_port "$p" || continue
            rate="$(sed -n 's/^# x-ui-meko-rate=//p' "$f" | head -n 1)"
            burst="$(sed -n 's/^# x-ui-meko-burst=//p' "$f" | head -n 1)"
            if [[ -n "$rate" ]]; then
                rate="$(normalize_rate "$rate")" || {
                    log "Invalid per-port MEKO rate for $p; using global rate" >&2
                    rate="$RATE"
                }
            else
                rate="$RATE"
            fi
            if [[ -n "$burst" ]]; then
                burst="$(normalize_burst "$burst")" || {
                    log "Invalid per-port MEKO burst for $p; using global burst" >&2
                    burst="$BURST"
                }
            else
                burst="$BURST"
            fi
            printf '%s %s %s\n' "$((10#$p))" "$rate" "$burst"
        done
        shopt -u nullglob
    fi
}

collect_ports() {
    collect_port_policies | awk '$1 ~ /^[0-9]+$/ && $1>=1 && $1<=65535 {print $1}' | sort -nu
}

load_port_policies() {
    ports=()
    port_rates=()
    port_bursts=()
    local p rate burst
    while read -r p rate burst; do
        valid_port "$p" || continue
        ports+=("$p")
        port_rates+=("$rate")
        port_bursts+=("$burst")
    done < <(collect_port_policies | sort -n -k1,1 -u)
}

ensure_u32() {
    if ! iptables -m u32 --help >/dev/null 2>&1; then
        modprobe xt_u32 >/dev/null 2>&1 || true
    fi
    iptables -m u32 --help >/dev/null 2>&1
}

remove_jump_all() {
    local table="$1" parent="$2" child="$3"
    while iptables -t "$table" -C "$parent" -j "$child" 2>/dev/null; do
        iptables -t "$table" -D "$parent" -j "$child" 2>/dev/null || break
    done
}

# A firewall manager may insert rules ahead of MEKO after it was applied.
# Merely checking that the jump exists is therefore insufficient: an earlier
# ACCEPT can bypass the filter. Healthy means exactly one jump, at rule #1.
jump_is_first_unique() {
    local table="$1" parent="$2" child="$3"
    iptables -t "$table" -L "$parent" --line-numbers -n 2>/dev/null | awk -v child="$child" '
        $1 ~ /^[0-9]+$/ && $2 == child {
            count++
            if ($1 == 1) first = 1
        }
        END { exit !(count == 1 && first == 1) }
    '
}

ensure_chain() {
    local table="$1" chain="$2"
    iptables -t "$table" -N "$chain" 2>/dev/null || true
    iptables -t "$table" -F "$chain"
}

remove() {
    if command -v nft >/dev/null 2>&1; then
        nft delete table inet xui_telemt_meko 2>/dev/null || true
    fi
    type -P iptables >/dev/null 2>&1 || return 0
    remove_jump_all filter INPUT "$FILTER_CHAIN"
    remove_jump_all mangle PREROUTING "$MARK_CHAIN"
    iptables -t filter -F "$FILTER_CHAIN" 2>/dev/null || true
    iptables -t filter -X "$FILTER_CHAIN" 2>/dev/null || true
    iptables -t mangle -F "$MARK_CHAIN" 2>/dev/null || true
    iptables -t mangle -X "$MARK_CHAIN" 2>/dev/null || true
    log "MEKO V3 rules removed"
}

# Native nftables needs no xt_u32 kernel module and applies one atomic batch.
# Payload offsets are relative to TCP, so the same iOS signature handles IPv6.
# Accepted SYNs return to the host firewall; its access policy is preserved.
apply_nft() {
    local port rate burst i
    local table_present=0

    # Replace the owned table in ONE nft transaction. An independent
    # "delete table" followed by "add table" would briefly remove protection;
    # plain "add table" fails on every subsequent update. A rejected batch
    # leaves the previous table intact.
    if nft list table inet xui_telemt_meko >/dev/null 2>&1; then
        table_present=1
    fi
    {
        if (( table_present )); then
            echo 'delete table inet xui_telemt_meko'
        fi
        echo 'add table inet xui_telemt_meko'
        echo 'add set inet xui_telemt_meko ports { type inet_service; }'
        printf 'add element inet xui_telemt_meko ports { %s }\n' "$(IFS=,; echo "${ports[*]}")"
        echo 'add chain inet xui_telemt_meko input { type filter hook input priority -10; policy accept; }'
        echo 'add rule inet xui_telemt_meko input tcp dport @ports tcp flags & (fin | syn | rst | ack) == syn @th,96,32 & 0x000fffff == 0x0002ffff @th,160,16 0x0204 @th,192,16 0x0103 @th,224,24 0x010108 @th,320,32 0x04020000 counter return'
        for i in "${!ports[@]}"; do
            port="${ports[$i]}"
            rate="${port_rates[$i]}"
            burst="${port_bursts[$i]}"
            printf 'add rule inet xui_telemt_meko input tcp dport %s tcp flags & (fin | syn | rst | ack) == syn meter tm4_%s { ip saddr timeout 60s limit rate over %s burst %s packets } counter reject with tcp reset\n' "$port" "$port" "$rate" "$burst"
            printf 'add rule inet xui_telemt_meko input tcp dport %s tcp flags & (fin | syn | rst | ack) == syn meter tm6_%s { ip6 saddr timeout 60s limit rate over %s burst %s packets } counter reject with tcp reset\n' "$port" "$port" "$rate" "$burst"
        done
    } | nft -f -
}

apply() {
    # Disabling must always work, even when old rate/burst settings are invalid.
    mapfile -t ports < <(collect_ports)
    # The selected inbound's switch wins over obsolete global service
    # settings (systemd passes those as environment variables). An explicit
    # TELEMT_PORTS override is reserved for administrative/test commands.
    if [[ ! -v TELEMT_PORTS ]]; then
        if (( ${#ports[@]} > 0 )); then ENABLED=1; else ENABLED=0; fi
    fi
    if ! enabled; then
        remove
        log "MEKO V3 is disabled by panel settings"
        return 0
    fi

    normalize_limits
    load_port_policies

    if (( ${#ports[@]} == 0 )); then
        remove
        log "No active Telemt MTProto ports found; rules left clean"
        return 0
    fi

    if command -v nft >/dev/null 2>&1; then
        if apply_nft; then
            # Remove only the legacy managed chains, leaving the new nft table.
        if type -P iptables >/dev/null 2>&1; then
            remove_jump_all filter INPUT "$FILTER_CHAIN"
            remove_jump_all mangle PREROUTING "$MARK_CHAIN"
            iptables -t filter -F "$FILTER_CHAIN" 2>/dev/null || true
            iptables -t filter -X "$FILTER_CHAIN" 2>/dev/null || true
            iptables -t mangle -F "$MARK_CHAIN" 2>/dev/null || true
            iptables -t mangle -X "$MARK_CHAIN" 2>/dev/null || true
        fi
            log "MEKO V3 nftables applied to IPv4/IPv6 ports: ${ports[*]}"
            return 0
        fi
        # Do not install another limiter over a pre-existing nft limiter.
        # This would silently double-filter traffic and obscure why users
        # cannot connect. Leave the old atomic nft rules in place instead.
        if nft list table inet xui_telemt_meko >/dev/null 2>&1; then
            log "Existing MEKO nft table could not be updated; refusing iptables fallback" >&2
            return 1
        fi
    fi
    type -P iptables >/dev/null 2>&1 || { log "nftables or iptables is required"; exit 1; }
    ensure_u32 || { log "xt_u32 is not available; MEKO V3 cannot be enabled"; exit 1; }

    ensure_chain filter "$FILTER_CHAIN"
    ensure_chain mangle "$MARK_CHAIN"

    # Keep exactly one jump and make it the first rule. This is intentionally
    # idempotent and also repairs ordering changed by ufw/firewalld/scripts.
    remove_jump_all filter INPUT "$FILTER_CHAIN"
    remove_jump_all mangle PREROUTING "$MARK_CHAIN"
    iptables -t mangle -I PREROUTING 1 -j "$MARK_CHAIN"
    # Upstream MEKO used INPUT position 2 only because it first inserted its
    # own global SSH ACCEPT rule at position 1. 3x-ui intentionally does not
    # mutate SSH policy, so the MEKO chain itself must run before any existing
    # INPUT ACCEPT rule or Telemt SYN packets could bypass the fix entirely.
    iptables -t filter -I INPUT 1 -j "$FILTER_CHAIN"

    local port rate burst i
    for i in "${!ports[@]}"; do
        port="${ports[$i]}"
        rate="${port_rates[$i]}"
        burst="${port_bursts[$i]}"
        valid_port "$port" || continue

        # Preserve every unrelated fwmark bit. MEKO owns only bit 0x400.
        iptables -t mangle -A "$MARK_CHAIN" -p tcp --dport "$port" -m u32 --u32 "$U32_FILTER" \
            -j MARK --set-xmark "$MARK/$MARK"
        # Passing the limiter must still honor the host firewall, as nft does.
        iptables -t filter -A "$FILTER_CHAIN" -p tcp --dport "$port" --syn -m mark --mark "$MARK/$MARK" -j RETURN

        # xt_hashlimit stores names in an IFNAMSIZ-sized field (15 visible
        # characters on Linux). Keep the per-port name short even for 65535.
        iptables -t filter -A "$FILTER_CHAIN" -p tcp --dport "$port" --syn \
            -m hashlimit --hashlimit-name "tm_${port}" --hashlimit-mode srcip \
            --hashlimit-upto "$rate" --hashlimit-burst "$burst" \
            --hashlimit-htable-expire 60000 --hashlimit-htable-size 32768 -j RETURN
        iptables -t filter -A "$FILTER_CHAIN" -p tcp --dport "$port" --syn \
            -j REJECT --reject-with tcp-reset
    done

    if command -v nft >/dev/null 2>&1; then
        nft delete table inet xui_telemt_meko 2>/dev/null || true
    fi
    log "MEKO V3 applied to Telemt MTProto ports: ${ports[*]} (rate=${RATE}, burst=${BURST})"
}

status() {
    local installed=false backend=none rules="" mark_rules="" port
    local -a applied_ports=() verified_ports=()
    local verified_all=true

    # An existing table or a REJECT alone is not proof that selected ports
    # are protected. Verify both address families and the iOS bypass.
    if command -v nft >/dev/null 2>&1 &&
       nft list table inet xui_telemt_meko >/dev/null 2>&1; then
        rules="$(nft -n list chain inet xui_telemt_meko input 2>/dev/null || true)"
        backend=nftables
        mapfile -t applied_ports < <(nft -n list set inet xui_telemt_meko ports 2>/dev/null | awk '
            /elements = {/ { inside=1; sub(/^.*elements = {/, "") }
            inside { end=($0 ~ /}/); gsub(/[{},]/, " "); for (i=1; i<=NF; i++) if ($i ~ /^[0-9]+$/) print $i; if (end) exit }
        ' | sort -n)
        # A detached regular chain can contain all expected rules while
        # protecting no traffic. Require our base-chain hook and priority.
        grep -Eq 'type filter hook input priority (-10|filter - 10); policy accept;' <<< "$rules" || verified_all=false
        # All iOS signature offsets and RETURN must occur on the same rule.
        # nft may normalize masks and flag order when listing rules.
        if ! grep -F 'tcp dport @ports' <<< "$rules" |
             grep -F '@th,96,32' | grep -F '@th,160,16' |
             grep -F '@th,192,16' | grep -F '@th,224,24' | grep -F '@th,320,32' |
             grep -Eq 'counter( packets [0-9]+ bytes [0-9]+)? return'; then
            verified_all=false
        fi
        for port in "${applied_ports[@]}"; do
            # Verify the actual IPv4 AND IPv6 reject rules for this port.
            # A named meter without a matching reject action is insufficient.
            if grep -F "tcp dport ${port} " <<< "$rules" |
                   grep -F "meter tm4_${port} " | grep -F "ip saddr" | grep -Fq "reject with tcp reset" &&
               grep -F "tcp dport ${port} " <<< "$rules" |
                   grep -F "meter tm6_${port} " | grep -F "ip6 saddr" | grep -Fq "reject with tcp reset"; then
                verified_ports+=("$port")
            else
                verified_all=false
            fi
        done
        if [[ "$verified_all" == true && ${#applied_ports[@]} -gt 0 ]]; then
            installed=true
        fi
    elif type -P iptables >/dev/null 2>&1 &&
         iptables -t filter -S "$FILTER_CHAIN" >/dev/null 2>&1; then
        backend=iptables
        if ! jump_is_first_unique filter INPUT "$FILTER_CHAIN" ||
           ! jump_is_first_unique mangle PREROUTING "$MARK_CHAIN"; then
            verified_all=false
        fi
        rules="$(iptables -t filter -S "$FILTER_CHAIN" 2>/dev/null || true)"
        mark_rules="$(iptables -t mangle -S "$MARK_CHAIN" 2>/dev/null || true)"
        mapfile -t applied_ports < <(
            printf '%s\n' "$rules" |
                awk '{ for (i=1; i<=NF; i++) if ($i=="--dport") print $(i+1) }' |
                awk '/^[0-9]+$/ { seen[$1]=1 } END { for (p in seen) print p }' |
                sort -n
        )
        for port in "${applied_ports[@]}"; do
            if grep -F " --dport ${port} " <<< "$rules" | grep -Fq -- "--hashlimit-name tm_${port} " &&
               grep -F " --dport ${port} " <<< "$rules" | grep -Fq -- "-j REJECT" &&
               grep -F " --dport ${port} " <<< "$mark_rules" | grep -Fq -- "-m u32"; then
                verified_ports+=("$port")
            else
                verified_all=false
            fi
        done
        if [[ "$verified_all" == true && ${#applied_ports[@]} -gt 0 ]]; then
            installed=true
        fi
    fi

    mapfile -t ports < <(collect_ports)
    if [[ ! -v TELEMT_PORTS ]]; then
        if (( ${#ports[@]} > 0 )); then ENABLED=1; else ENABLED=0; fi
    fi
    printf 'enabled=%s\n' "$(enabled && echo true || echo false)"
    printf 'installed=%s\n' "$installed"
    printf 'backend=%s\n' "$backend"
    printf 'ports=%s\n' "$(IFS=,; echo "${ports[*]:-}")"
    printf 'applied_ports=%s\n' "$(IFS=,; echo "${applied_ports[*]:-}")"
    printf 'verified_ports=%s\n' "$(IFS=,; echo "${verified_ports[*]:-}")"
    printf 'rate=%s\n' "$RATE"
    printf 'burst=%s\n' "$BURST"
}

case "${1:-apply}" in
    apply) apply ;;
    remove) remove ;;
    status) status ;;
    *) echo "Usage: $0 {apply|remove|status}" >&2; exit 2 ;;
esac
