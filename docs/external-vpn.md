# Pingtunnel and TrustTunnel inbounds

Both protocols are independent processes managed by the panel, for local Linux inbounds. They are not Xray or sing-box protocols and cannot be deployed to a sub-node. The release packages include the official Linux amd64/arm64 binaries: Pingtunnel 2.10 and TrustTunnel endpoint 1.1.0. A source build must place `pingtunnel` and `trusttunnel_endpoint` in the panel's binary folder or on `PATH`.

## Pingtunnel

Create a Pingtunnel inbound. Its port is always `0` because it listens for ICMP, not TCP or UDP. Leave the key and encryption secret blank to have the panel generate and persist them. Only one Pingtunnel inbound can be enabled on a server. After saving, open the inbound editor to copy the generated client JSON, set `server` to the public IP or domain, and run the official client with `pingtunnel -c client.json`. Its local SOCKS5 listener is `127.0.0.1:1080`. Opening ICMP at the host firewall and raw socket permission are required. The upstream guide mentions disabling the kernel's ICMP echo response as an optional host setting; the panel does not alter that global setting.

Pingtunnel authenticates with a shared numeric key and encryption secret. It has no separate user accounts or per-user traffic metrics. Changing either credential disconnects all clients. Generic Pingtunnel links are not added to subscriptions because the official client uses a JSON configuration instead of a URI.

## TrustTunnel

Create a TrustTunnel inbound with a TCP/UDP port, then add clients in the Clients panel. The panel generates client passwords. Set `hostname` to the TLS name the client will use; if certificate and private key paths are empty, the panel creates a self-signed certificate for that name under `bin/externalvpn/<inbound-id>/`. The official endpoint exporter embeds that certificate in `tt://?` links, which appear in raw subscriptions and client share views. For a publicly trusted certificate, set both PEM paths and manage renewal separately. The panel writes `vpn.toml`, `hosts.toml`, and `credentials.toml` with private file permissions and starts `trusttunnel_endpoint` automatically. Only active clients are written to the credentials file.

The endpoint's per-user metrics are bound to a random loopback port. The panel polls them every 10 seconds for client and inbound traffic accounting, so traffic limits and expirations can disable a client on the next poll. JSON/Clash/sing-box subscriptions fall back to raw links when TrustTunnel is present because those formats have no equivalent native outbound in this panel. Use an official TrustTunnel client that accepts `tt://?` deep links.

Official documentation: [Pingtunnel usage](https://github.com/esrrhs/pingtunnel/blob/master/USAGE.md), [TrustTunnel endpoint configuration](https://github.com/TrustTunnel/TrustTunnel/blob/master/CONFIGURATION.md), and [TrustTunnel client export](https://github.com/TrustTunnel/TrustTunnel#export-client-configuration).

## VetroffProxy (inbound and managed outbound)

VetroffProxy is an external sidecar on Linux amd64/arm64, not a built-in Xray or
sing-box protocol. Install its **full** `vetroff-necko` bundle using the panel's
component installer; the server and client modes use the same executable.

**Inbound:** Create a `vetroff` inbound, choose a TLS hostname/certificate and
add users. The panel manages `vetroff server` and generates per-user
`vetroff://` connection links. The server forwards authenticated TCP and,
when enabled, UDP traffic to its configured destinations.

**Outbound:** Add a `vetroff (managed)` outbound in the outbound editor, paste
one of the generated `vetroff://` links into its `link` field, and enable
`enableUDP` only when the remote inbound supports UDP. The panel starts
`vetroff client --link-file` with a private, mode-0600 connection file and a
SOCKS5 listener bound **only** to `127.0.0.1`. Xray and sing-box use that
local SOCKS bridge under the original outbound tag, so routing rules, selectors
and chains can reference it by tag.

Example saved outbound (replace the placeholder with a complete link):

```json
{
  "tag": "vetroff-out",
  "protocol": "vetroff",
  "settings": {
    "link": "vetroff://<base64url-profile>",
    "enableUDP": false,
    "udpStream": false
  }
}
```

The link contains credentials: do not log it or put it into a public repository.
`udpStream` requires `enableUDP`. The client rejects UDP with Bond mode.
Updating the component stops existing clients and the managed supervisor
restarts them using the new bundle; uninstalling stops clients and removes their
restart plans. Existing inbound settings and outbound templates are not deleted.
Client/server compatibility and end-to-end networking require the matching
VetroffProxy release. See [upstream setup](https://github.com/SawaMEN/VetroffProxy/blob/main/docs/START.md).
