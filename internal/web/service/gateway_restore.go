package service

import (
	"fmt"
	"os/exec"
	"strings"
)

func gatewayShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (s *GatewayNetworkService) gatewayNetworkUnits() (string, string, string, error) {
	paths := s.networkPaths()
	// Resolve paths on this distribution rather than assuming /usr/sbin.
	commands := map[string]string{}
	for _, name := range []string{"ip", "nft", "sysctl", "ss", "flock", "bash"} {
		path, err := exec.LookPath(name)
		if s.run != nil {
			path = "/usr/bin/" + name
			err = nil
		}
		if err != nil {
			return "", "", "", err
		}
		commands[name] = gatewayShellQuote(path)
	}
	script := strings.NewReplacer(
		"@STATE@", gatewayShellQuote(paths.state), "@RULES@", gatewayShellQuote(paths.nft),
		"@RP@", gatewayShellQuote(paths.rpFilter), "@LOCK@", gatewayShellQuote(paths.state+".lock"),
		"@IP@", commands["ip"], "@NFT@", commands["nft"], "@SYSCTL@", commands["sysctl"],
		"@SS@", commands["ss"], "@FLOCK@", commands["flock"],
	).Replace(gatewayRestoreScript)
	unit := func(mode string) string {
		dependencies := "After=network-online.target\n"
		if mode == "firewall" {
			dependencies = "After=network-online.target nftables.service x-ui.service xui-gateway-routing.service\nRequires=xui-gateway-routing.service\nWants=x-ui.service\n"
		}
		return fmt.Sprintf("[Unit]\nDescription=3X-UI Gateway %s\nWants=network-online.target\n%s\n[Service]\nType=oneshot\nExecStart=%s %s %s\nTimeoutStartSec=30\nRemainAfterExit=yes\n\n[Install]\nWantedBy=multi-user.target\n", mode, dependencies, strings.Trim(commands["bash"], "'"), paths.restoreScript, mode)
	}
	return unit("routing"), unit("firewall"), script, nil
}

const gatewayRestoreScript = `#!/bin/bash
set -euo pipefail
export LC_ALL=C
STATE=@STATE@
RULES=@RULES@
RP=@RP@
[[ -f "$STATE" ]] || exit 0
exec 9>"$STATE.lock"
@FLOCK@ -w 25 9
[[ -f "$STATE" ]] || exit 0
grep -qx 'NETWORK_ENABLED=0' "$STATE" && exit 0
lan=$(sed -n 's/^LAN_IF=//p' "$STATE")
[[ "$lan" =~ ^[a-zA-Z0-9_.:-]+$ ]] || { echo 'Invalid Gateway LAN interface' >&2; exit 1; }
if [[ "${1:-firewall}" == routing ]]; then
    # Check aliases numerically so named table 100 cannot be silently replaced.
    numeric_rules=$(@IP@ -N -4 rule show)
    owned='^[0-9]+:[[:space:]]+from all fwmark 0x40/0xc0 (lookup|table) 100( proto [[:alnum:]_-]+)?[[:space:]]*$'
    if printf '%s\n' "$numeric_rules" | grep -E '(lookup|table) 100( |$)' | grep -vE "$owned" >/dev/null; then
        echo 'Gateway routing table is owned by another rule' >&2; exit 1
    fi
    routes=$(@IP@ -4 route show table 100 2>&1) || {
        [[ "$routes" == *'FIB table does not exist'* ]] || { echo "$routes" >&2; exit 1; }
        routes=''
    }
    if [[ -n "$routes" ]] && printf '%s\n' "$routes" | grep -vE '^local default dev lo( |$)' >/dev/null; then
        echo 'Gateway routing table contains a foreign route' >&2; exit 1
    fi
    @IP@ -4 route replace local default dev lo table 100
    mapfile -t policy_lines < <(printf '%s\n' "$numeric_rules" | grep -E "$owned")
    if [[ "${#policy_lines[@]}" -gt 1 ]]; then
        for line in "${policy_lines[@]}"; do
            @IP@ -4 rule del priority "${line%%:*}" from all fwmark 0x40/0xc0 table 100
        done
        policy_lines=()
    fi
    if [[ "${#policy_lines[@]}" -eq 0 ]]; then @IP@ -4 rule add fwmark 0x40/0xc0 table 100; fi
    exit 0
fi
[[ -s "$RULES" && -s "$RP" ]] || { echo 'Gateway recovery files are missing' >&2; exit 1; }
for attempt in {1..100}; do
    tcp=$(@SS@ -H -l -n -t 'sport = :52345')
    udp=$(@SS@ -H -l -n -u 'sport = :52345')
    [[ -n "$tcp" && -n "$udp" ]] && break
    [[ "$attempt" -lt 100 ]] || { echo 'Gateway listener is not ready' >&2; exit 1; }
    sleep 0.1
done
while read -r key value; do
    [[ "$key" =~ ^net/ipv4/conf/[a-zA-Z0-9_.:-]+/rp_filter$ && "$value" =~ ^[012]$ ]] || { echo 'Invalid Gateway rp_filter snapshot' >&2; exit 1; }
done < "$RP"
tables=$(@NFT@ list tables)
batch=$(mktemp)
trap 'rm -f "$batch"' EXIT
for table in 'inet xui_gateway' 'ip xui_gateway_nat'; do
    if printf '%s\n' "$tables" | grep -qx "table $table"; then echo "delete table $table" >> "$batch"; fi
done
cat "$RULES" >> "$batch"
@NFT@ -c -f "$batch"
if [[ "$(@SYSCTL@ -n net.ipv4.ip_forward)" != 1 ]]; then @SYSCTL@ -w net.ipv4.ip_forward=1; fi
while read -r key value; do
    [[ -f "/proc/sys/$key" ]] || continue
    if [[ "$key" == net/ipv4/conf/all/rp_filter || "$key" == "net/ipv4/conf/$lan/rp_filter" ]]; then value=0; fi
    @SYSCTL@ -w "$key=$value"
done < "$RP"
@NFT@ -f "$batch"
`
