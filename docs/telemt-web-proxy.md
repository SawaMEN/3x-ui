# WEB Proxy: transports and HTTPS frontend

Telemt is an optional runtime for the existing **MTProto** inbound protocol in
3x-ui. To install it, use **Настройки → Обновления пакетов и ядер Linux**.
There is **no manual Telemt start/stop menu**. The panel manages service
lifecycles from enabled MTProto inbounds:

- **Ordinary MTProto**: in **Входящие → Создать подключение → MTProto**, leave
  WEB Proxy disabled. The panel runs a supervised Telemt sidecar for the
  inbound; removing/disabling the inbound stops that sidecar. Links are
  `tg://proxy` in the usual client, QR and subscription workflows. The optional
  **Fix by MEKO V3** switch is bound to this inbound's TCP port and defaults
  to off. It does not apply to any WEB Proxy frontend/backend or other ports.
- **WEB Proxy**: in the same MTProto form, enable **WEB Proxy (HTTPS)**, provide
  a public domain and choose nginx, Caddy or HAProxy and HTTP carrier(s).
  The WEB frontend and its Telemt backend are provisioned from the inbound.
  The individual client links are `tg://webproxy`, generated in the same
  client, QR and subscription interfaces, using each MTProto client's secret.
  Disabling/deleting the last managed WEB inbound stops its frontend/backend.
  WEB mode is currently supported only on the local server, with **one**
  managed WEB inbound per server because its HTTPS frontend owns port 443.

### WEB: domain and port selection

The form sets the public port to **443** automatically; the loopback backend
port is allocated separately. The inbound's general listen address does not
restrict the HTTPS frontend. Existing WEB records with another stored port
still reserve TCP 443 in conflict checks.

The panel domain is suggested when available. You can use it or a separate
public domain: A/AAAA records must point to this server and the certificate
must cover the chosen name. The form preserves a previously entered domain.
One domain can serve WEB on 443 and the panel on another port, for example
`https://web.example.com:2053/panel-path/`. If the panel itself already listens
on 443, move it using panel settings first. This integration does not migrate
the panel listener or route its login UI through the WEB decoy vhost. Existing
reverse-proxy vhosts for the same domain need administrator reconciliation.

Use **Check WEB Proxy readiness** to refresh the runtime and port diagnostics.
A successful bind check does not verify DNS or external firewall reachability.
Permit TCP 443 in host/provider firewalls and TCP 80 when HTTP certificate
issuance/renewal is used. DNS and provider firewalls require access to their
respective control planes and are not changed by the panel.

Periodic inbound reconciliation also restarts a stopped managed HTTPS
frontend when its Telemt backend remains active.

### Ordinary MTProto: Fix by MEKO V3

Fix by MEKO is **opt-in per ordinary MTProto inbound** and targets the
inbound's listening TCP port. The **WEB Proxy** transport never uses these
rules. Rate (SYN/minute per source IP) and burst can be tuned on each inbound;
default policy is 54/minute, burst 1. MEKO is a connection-attempt limiter, **not
a substitute for a general host firewall or a guarantee of reachability from a
filtered network**.

The panel's MEKO helper owns a dedicated nftables table
(`inet xui_telemt_meko`) and falls back to managed iptables chains when
nftables is unavailable. On an existing nftables installation, updates replace
the owned table in **one atomic transaction**; an unsuccessful replacement does
not install a second limiter via iptables. The iptables fallback currently
filters IPv4 only, so install nftables when ordinary MTProto accepts IPv6.
Neither backend changes unrelated SSH rules or other nftables tables.

The MEKO systemd unit is enabled only while at least one opted-in ordinary
MTProto inbound exists. Port changes, disable/delete, and changes in per-inbound
limits trigger reconciliation. If a sidecar fails to start, its generated port
selection is removed rather than applying MEKO to a service that might later
reuse the port. The inbound form displays selected versus actually applied
ports, reports firewall errors, and offers **Retry MEKO** after saving.

Do not enable Telemt's built-in `server.listeners.synlimit` on a port that is
already managed by the external MEKO helper: stacking two independent SYN
limiters may reject legitimate connections. MEKO's iOS-like TCP signature is a
heuristic and may vary with OS and network path. Validate rate/burst against
real traffic and check both IPv4 and IPv6 before deployment.

New WEB inbounds will not take over a previously configured independent
WEB Proxy automatically. The legacy configuration is preserved, and the old
WEB frontend must be disabled before a new inbound can claim port 443.
Legacy standalone Telemt credentials are not deleted or automatically migrated.

Supported carriers are `https`, `https-lanes`, `websocket` and
`websocket-lanes`. Selection/negotiation requires Telemt 3.5.4 or newer. An empty
list uses only the primary carrier. Clients without negotiation support use the
primary carrier. Existing installations retain their single HTTPS transport
until explicitly reconfigured; new installations suggest WebSocket lanes,
WebSocket and HTTPS in that order.

Caddy and HAProxy use the panel-owned
`3x-ui-telemt-web-front.service`, with private configuration under
`/etc/x-ui/telemt-web-front/`. Existing global Caddy/HAProxy configuration is not
rewritten. A newly installed package's default service is disabled before the
panel-owned service starts. nginx continues using the existing managed vhost.
Caddy/HAProxy setup requires systemd; a missing package is installed through the
host package manager when available. Ordinary HTTP reverse-proxy builds are
sufficient for WEB Proxy; a Caddy layer4 plugin is not required for this path.

The chosen frontend terminates TLS on port 443 and sends HTTP/WebSocket to the
loopback-only Telemt WEB listener. Forwarded client addresses are overwritten
at that trusted boundary. Imported Hiddify subscription paths route to the
panel, preserving path, query and Host. An HTTPS panel connection validates its
configured certificate and certificate name, including self-signed setups.

An existing valid panel certificate or Let's Encrypt certificate is reused.
For a new Caddy/HAProxy certificate, certbot standalone needs free port 80;
nginx retains its webroot issuance path and keeps the managed HTTP challenge
route for renewals. Switching a panel HTTP certificate between webroot and
standalone uses `certbot reconfigure` (Certbot 2.3+), tests the new method against
the staging CA, and restores the previous renewal configuration on failure.
External DNS/custom-webroot certificates retain their existing renewal method. HAProxy's combined PEM is
rebuilt by the deploy hook; Caddy is force-reloaded to pick up renewed files.
The default WEB frontend does not promise a particular post-quantum TLS group;
that depends on the installed Caddy/HAProxy/OpenSSL build.

Switching frontend preserves the Telegram secret and link but may briefly
interrupt connections. Mutating WEB operations are serialized, the native
frontend configuration is validated before activation, and setup or backend
readiness failure restores the previous WEB configuration and Telemt runtime.
Turning WEB Proxy off removes the managed frontend configuration and stops the
panel-owned Caddy/HAProxy service. Existing unrelated frontend services are not
stopped. Required inbound ports must still be permitted by the host firewall.

The enable endpoint remains `POST /panel/api/telemt/webproxy/enable` and accepts:

```json
{
  "domain": "web.example.com",
  "frontend": "caddy",
  "carrier": "https",
  "carriers": ["websocket-lanes", "websocket", "https"]
}
```

Omitted transport/frontend fields preserve existing choices. Passing
`"carriers": []` explicitly disables negotiation.

Native tests exercise HTTPS routing, certificate trust, unknown-host rejection,
forwarding-header replacement and bidirectional WebSocket transfer with Caddy
and HAProxy. They also run the generated carrier configuration against the
official Telemt binary. These checks do not measure accessibility from a
particular mobile operator.
