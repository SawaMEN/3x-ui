# WEB Proxy: transports and HTTPS frontend

## Quick setup and terminology

In **Inbounds → MTProto**, enable **WEB Proxy**, enter a public domain and
leave **nginx** and the primary **HTTPS** transport selected if unsure. Save
the inbound with at least one enabled client. The panel installs the HTTPS
server when needed, reuses or obtains a certificate, creates the cover site,
allocates the private Telemt port and manages the services. Open the website
from the form after saving; use the client's `tg://webproxy` link in a Telegram
version that supports WEB Proxy.

| Interface setting                 | Meaning                                                                                                                                                                                     |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| HTTPS server (frontend)           | nginx, Caddy or HAProxy accepts encrypted connections on public TCP 443 and forwards them to private Telemt. The panel configures the selected program.                                     |
| Primary transport (WEB carrier)   | How Telegram data travels inside HTTPS. `https` is the compatibility fallback, including native iOS clients without negotiation metadata.                                                   |
| `websocket`                       | One persistent WebSocket carries the logical streams.                                                                                                                                       |
| `https-lanes` / `websocket-lanes` | Independent HTTPS sequencing/polling or WebSocket connections for individual logical streams. Requires compatible clients.                                                                  |
| Transport priority / negotiation  | Clients supporting negotiation try the selected transports in order at session startup. Other clients use the primary transport. Empty means no negotiation; active sessions never migrate. |
| Local backend                     | Telemt's private HTTP listener on loopback; it must not be exposed to the Internet.                                                                                                         |
| Cover site / decoy                | Ordinary browser requests receive a regular website; authenticated Telegram requests enter the proxy. A website does not guarantee resistance to detection or blocking.                     |

### Automatic cover website

A cover site is part of this WEB deployment. **No separate hosting or extra
public listener is required.** Telemt serves a local static snapshot, while
the selected HTTPS server forwards the full WEB scope to Telemt. Do not add a
competing `location /` serving files directly in nginx: that would intercept
Telegram's bridge/session requests.

New installations receive a responsive, self-contained notes website. It has
no external scripts, fonts, images or analytics. The form explains this and
offers a link to open the public site. Existing non-empty regular `index.html`
files, including the earlier built-in welcome page and customized sites, are
preserved on reconfiguration, frontend changes and domain changes. Creation
publishes a complete file without truncating an existing site. Invalid empty,
directory or symlink indexes stop setup with an error.

For a custom site, place its files in `/var/lib/x-ui/telemt-web/` with a
non-empty `index.html`, then reapply WEB settings so Telemt rebuilds its static
snapshot. The built-in page is used only when `index.html` is absent. Files
must be readable by Telemt. Changing a saved domain preserves site content;
update any domain-specific text in a custom or previously generated page.

### Optional ready-made catalogs

Leave **Cover website: optional customization** closed for fully automatic
setup. To choose a ready-made site, select **Choose from ready-made catalogs**,
choose the **Catalog** and **Ready-made page**, then save/apply. The panel
installs the page with all its assets and applies the normal readiness checks
and rollback. No file uploads, direct URLs, API keys or separate hosting are
required. The same controls appear in both the inbound and standalone forms.

| Catalog             | Ready-made pages                                 |
| ------------------- | ------------------------------------------------ |
| 3x-ui pages         | Personal journal, Technology, Travel, Nature; Coffee house, Bakery, Photography, Architecture, Reading room, Ceramics studio, Garden journal, Night sky, Kitchen notebook, Cycling journal, Listening room, Coworking space; Video magazine, Cinema journal, Games magazine, Learning library |
| MTProxyL collection | Simple maintenance page, Cat Runner, Meko Runner |

The 20 built-in themes are available in both catalog and generated modes.
Generated mode accepts an optional title and description. Video magazine,
Cinema journal, Games magazine and Learning library imitate content-rich media
portals with illustrated covers, six stories, curated lists, local search and
category filters. Story links open the corresponding reading-room entry.
They do not host videos or download media: illustrations are inline SVG and
all CSS/JavaScript is served locally. The sites also remain readable without
JavaScript. Layouts adapt to mobile screens and respect reduced-motion settings.

The catalogs ship inside the panel binary, so activation requires no download
from an external catalog. MTProxyL sources are pinned to `6d7021c` and retain
the MIT license; see `internal/web/service/telemt_sites/mtproxyl/README.md`.
The file-manager template is excluded because it depends on remote Tailwind
and Lucide scripts. Bundled games use local scripts/styles and system-font
fallbacks instead of fetching Google Fonts.

Telemt's static-site Content Security Policy requires local stylesheet and
script files. New automatic, generated and catalog pages therefore deploy
`site.css` and, where needed, `site.js` alongside `index.html`. No weakening of
Telemt's headers is required. Complete snapshots include all assets and reuse
identical content; a failed activation preserves the previously active site.
Existing automatic/custom sites are kept unchanged.

**Generate a themed page** remains available when a custom title and
introduction are wanted. This uses local templates for journal, technology,
travel or nature; there is no AI service or payment. Returning to Automatic
reconnects the preserved default directory. Changes apply after saving and may
briefly interrupt WEB sessions while Telemt rebuilds its static snapshot.

The old HTML upload and paste fields have been removed from the interface.
Previously saved HTML sites remain active and appear as **Previously installed
page**; choose another mode to replace them. The legacy `html` API format is
retained for compatibility. No arbitrary URL downloader is implemented.

`site` remains optional in enable/preflight requests. Omission preserves the
current standalone selection; `{"mode":"auto"}` restores the default. An
older inbound without `site` means Automatic. Catalog example:

```json
{
  "domain": "web.example.com",
  "frontend": "nginx",
  "carrier": "https",
  "carriers": [],
  "site": {
    "mode": "catalog",
    "catalog": "mtproxyl",
    "template": "catrunner"
  }
}
```

Catalog/template IDs are validated as fixed pairs. URLs, filesystem paths,
unknown catalogs and templates from another collection are rejected before
setup changes any files or services.

### Comparison with MTProxyL

Source review: `SawaMEN/3x-ui` at `d6fe19c`,
[MTProxyL](https://github.com/Liafanx/MTProxyL) at `6d7021c`, and
[Telemt WEB documentation](https://github.com/telemt/telemt/blob/9d5b896/docs/WEB/WEB_PROXY.en.md).
Both managers use Telemt's WEB implementation; MTProxyL is not a different
wire protocol.

| Capability                                       | 3x-ui                                                                                             | MTProxyL source reviewed                                                    |
| ------------------------------------------------ | ------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| HTTPS server, certificate and local WEB listener | Managed by the panel                                                                              | Managed by the setup wizard/CLI                                             |
| Carrier selection and ordered negotiation        | Inbound and Telemt forms                                                                          | Wizard/CLI settings                                                         |
| Cover website                                    | Automatic static site, two bundled catalogs or themed local generation; preserve existing content | Empty, static directory or HTTP origin; template, URL or local site sources |
| Ordinary MTProto and WEB                         | Separate managed inbounds; one local WEB inbound                                                  | WEB-only or combined; additional shared/split frontend layouts              |
| Readiness                                        | DNS/port/certificate preflight, private HTTP and public TLS checks, rollback                      | Listener/static-snapshot checks and managed restart when needed             |
| Site customization in this change                | Automatic default, seven ready-made pages in two catalogs and themed generation                   | More site-source and deployment choices                                     |

This integration keeps the existing one-WEB-inbound deployment model. DNS,
provider firewall changes, real certificate issuance and a Telegram connection
through the public domain still require validation on the target server. Local
builds and HTTP tests cannot establish reachability through a mobile operator.

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

The WEB setup dialog now performs a **read-only preflight** automatically on every
domain, frontend or carrier change, and offers **Check again**. It checks the
actual proposed domain rather than the previously saved WEB status. Enabling
WEB reruns the same preflight server-side before writing any configuration.

Preflight requires: a valid public FQDN; public DNS resolution (matching this
server's public network interface when one is assigned); an installed Telemt
binary of the required version; root privileges; an available local backend
port; TCP 443 available for the managed frontend (or already served by it);
no second Nginx HTTPS vhost for the same domain; a supported package manager
when an absent frontend or Certbot needs installation; and either a valid,
unexpired TLS certificate/private-key pair for the FQDN, or TCP 80 available
for automatic Let's Encrypt HTTP-01 issuance. Carriers other than the base
HTTPS transport require Telemt 3.5.4+.

If a normal Telemt listener is already configured on TCP 443, preflight
blocks WEB deployment: moving that native listener automatically would break
existing MTProxy links. Change its port in Telemt settings first. Without a
public FQDN, use ordinary MTProxy instead of Telegram Desktop WEB Proxy.

**External checks remain necessary:** DNS must point directly to the server
(not to a CDN), cloud/provider firewalls must permit inbound TCP 443 and TCP
80 for certificate issuance, and NAT deployments must forward these ports.
Local preflight cannot truthfully establish the remote reachability of a
public address. A successful preflight is a prerequisite check, not a promise
that an external Telegram client can connect.

Read-only endpoint: `POST /panel/api/telemt/webproxy/preflight` with the same
`domain`, `frontend`, `carrier` and `carriers` fields as the enable
endpoint. Its response contains `ready`, individual `checks` and
non-blocking `warnings`. The enable endpoint refuses a failed preflight.

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
