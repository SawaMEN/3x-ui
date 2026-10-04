# Gateway Mode

Gateway uses a dedicated transparent TCP/UDP listener on port 52345 (`in-tproxy`) in the selected Xray or sing-box core. Intercepted traffic follows the normal configured outbound routing. Gateway does not choose a proxy outbound for you.

## Host and LAN requirements

- Linux, root, systemd, iproute2 (`ip`, `ss`), nftables, sysctl, bash and flock.
- Kernel support for nftables TPROXY and policy routing.
- An active LAN interface with the entered IPv4 address actually assigned to it. LAN and optional WAN interfaces must differ.
- LAN clients must use this machine as their IPv4 gateway. Existing firewall policies must allow the forwarded traffic and marked traffic delivered to the transparent listener.
- Routing table 100, mark bits `0x40/0xc0`, port 52345 and tag `in-tproxy` must be available. Table 100 is refused if another rule or route owns it.

Interception covers IPv4 TCP/UDP arriving on the selected LAN interface from the configured LAN subnet. IPv6, traffic originating on the gateway itself, other LANs, local destinations and the configured private/reserved bypass ranges are not intercepted. NAT is installed only when a WAN interface is provided. Existing unrelated firewall tables are retained.

## Web panel and CLI

Start the selected core, then enable Gateway in Settings with the LAN interface, its IPv4 address and prefix. The panel creates the core-specific listener, waits for its TCP and UDP sockets, and configures forwarding, rp_filter, policy routing, nftables and boot restoration.

The CLI uses the same network service and selects the configured core:

```sh
x-ui gateway enable --lan-interface br-lan --lan-ip 192.168.50.1 --lan-prefix 24 --wan-interface eth0
x-ui gateway status
x-ui gateway disable
```

Omit `--wan-interface` when NAT is unnecessary. Enabling through the CLI restarts the panel service when it changes the listener template. Repeated enable repairs the saved network configuration; disable before changing LAN/WAN settings.

## Recovery and disabling

Status reports forwarding, rp_filter, the policy route, nftables, optional NAT, persistent files/services and both listener sockets. A template without Linux rules can be completed from the panel. A backup or partial template without a complete listener must be disabled before enabling again.

Disabling removes interception before changing the runtime. The original forwarding and rp_filter values are restored, and only Gateway-owned rules, routes and files are removed. User template edits are preserved. Recovery data remains if cleanup fails so disabling can be retried.

Core switches suspend interception, migrate the Gateway template, start the target runtime and restore networking after the listener becomes ready. Failures attempt to restore the previous core and configuration. Web requests, CLI changes and boot restoration share a lock.

Recovery files are stored under `/etc/x-ui`. Failed or suspended operations have `NETWORK_ENABLED=0`, preventing boot restoration from enabling incomplete interception. Corrupt recovery files are reported instead of being sourced as shell commands. A firewall table without its recovery state or an orphaned rp_filter snapshot requires administrator inspection; original forwarding settings cannot safely be inferred from missing data.

## Validation boundary

Regression tests simulate Linux command failures and cover activation, repeated repair, suspension, disabling, rollback, saved sysctl values, conflicts, sing-box config generation and restore-script syntax. They do not replace a real-server test of TCP/UDP interception and reboot restoration. Check both on the target kernel and distribution.
