# WEB Proxy: transports and HTTPS frontend

The Telemt page can enable or reconfigure WEB Proxy using **nginx**, **Caddy**
or **HAProxy**. nginx remains the default for existing installations.

1. Open **Telemt → Настроить Web Proxy** (or **Поднять Web Proxy**).
2. Enter a public domain pointing to this server and select an HTTPS frontend.
3. Select the primary/fallback carrier and, optionally, an ordered carrier list.
4. Use the up/down buttons to set attempt order and apply the settings.

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
patched Telemt binary. These checks do not measure accessibility from a
particular mobile operator.
