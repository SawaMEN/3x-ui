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
  to off. Optionally set a per-inbound **SYN rate/minute** and **burst**
  (defaults: 54/minute and burst 1). Changing these settings updates the
  firewall without restarting the Telemt process or disconnecting clients.
  The inbound editor displays the selected port, actual applied firewall
  backend and an error when the live rules do not match. Click **Refresh**
  after saving. It does not apply to WEB Proxy or other ports.

  MEKO is implemented by a separate host-owned firewall helper (nftables
  preferred, iptables/u32 fallback), not Telemt's built-in `synlimit`. These
  two limiters must not be enabled on the same listener simultaneously. The
  helper changes only its own netfilter table/chains, never generic SSH rules
  or unrelated VPN ports. Actual behavior depends on Linux kernel, netfilter
  capabilities, external firewall managers, client NAT and network conditions;
  rate limiting is not a guarantee against censorship or service blocking.
- **WEB Proxy**: in the same MTProto form, enable **WEB Proxy (HTTPS)**, provide
  a public domain and choose nginx, Caddy or HAProxy and HTTP carrier(s).
  The WEB frontend and its Telemt backend are provisioned from the inbound.
  The individual client links are `tg://webproxy`, generated in the same
  client, QR and subscription interfaces, using each MTProto client's secret.
  Disabling/deleting the last managed WEB inbound stops its frontend/backend.
  WEB mode is currently supported only on the local server, with **one**
  managed WEB inbound per server because its HTTPS frontend owns port 443.

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
