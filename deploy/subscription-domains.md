# Separate subscription domains and CDN

For the integrated automated gateway with Direct/CDN/Relay/subscription-only roles
and certificate lifetime monitoring, use **Settings → Domains and TLS**; see
[domains-and-tls.md](domains-and-tls.md). The options below remain available for
deployments that use their own external reverse proxy.

Configure **Settings → Subscription → Panel settings**:

- **Subscription domains**: select one or more enabled names from Domains and TLS,
  or enter externally managed names. The first name becomes the public origin;
  the remaining names become aliases. Selecting names enables **Use only selected
  domains**, which rejects other gateway domains and the internal listener hostname.
  Disable the restriction to restore the previous automatic domain allowance.
- **Public subscription port**: changes the port advertised in all subscription
  formats, independently of the internal subscription listener. For integrated
  domains, use the gateway public port configured in Domains and TLS; external
  proxies must forward the selected public port to the internal subscription port.
  Editing this field does not rebind the gateway or configure an external proxy.

- **Public subscription address**: `https://sub.example.com` (no path). The panel,
  subscription page and Telegram bot use this origin for raw, JSON and Clash URLs.
  Their existing path settings are appended automatically. This explicitly overrides
  the per-format URI settings while enabled; clearing it restores those settings.
- **Subscription domain**: the origin hostname, if Host filtering is required.
- **Additional subscription domains**: alternative allowed hostnames, separated by
  commas. The public address is allowed automatically. Aliases serve the same
  subscriptions; generated links use the public address as their canonical origin.
- **Subscription through CDN**: requires HTTPS and adds no-cache headers for
  personalized responses. The server does not trust CDN client-IP headers unless
  its peer is covered by the existing trusted-proxy configuration.

The subscription listener remains separate from the admin listener. These settings
never publish the panel on subscription domains and never change VPN node addresses.

## Reverse proxy

The settings page generates a Caddy site block for the public hostname and aliases.
Install Caddy on the same server, with public ports 80/443 available, and paste the
block into its Caddyfile. The generated block proxies to `127.0.0.1:<subPort>` and
preserves the original Host. Set **Subscription listening IP** to `127.0.0.1`.
If binding to another IP, adapt the generated upstream accordingly.

Use separate DNS records: `panel.example.com` for the panel and `sub.example.com`
for this Caddy site. Point the subscription record to the server. Caddy obtains
its public certificate automatically. If the subscription listener itself uses
HTTPS, its configured certificate must be valid and trusted by Caddy; the generated
transport sets the configured origin hostname as TLS server name.

When ports 80/443 are already occupied by nginx, HAProxy or a VPN inbound, add the
subscription virtual host to that existing gateway instead of starting a second
listener on those ports. Do not send subscription requests to the admin listener.

## Cloudflare / another CDN

Create the DNS record in the CDN account and enable proxying. Use Full (strict)
between the CDN and the TLS origin. Bypass cache and browser challenges for all
subscription, JSON, Clash and subscription-info requests. Preserve query strings,
User-Agent, Accept and X-HWID headers: formats and device limits depend on them.
Do not use Cache Everything or HTML minification on subscription responses.

The panel supplies the public address, accepted hosts and cache-control headers;
it does not create provider DNS records or change the provider account. The CDN
here carries subscription HTTP traffic. VPN transport through a CDN still requires
an appropriate inbound transport and its separate configuration.

## Domain website redirect and appearance

In **Settings → General**, enable domain website redirect and choose an HTTP/HTTPS
destination. Ordinary GET/HEAD browser requests are redirected with **302**:

- `https://example.com/123` → `https://destination.com/123`
- `https://example.com/a/b?q=1` → `https://destination.com/a/b?q=1`
- `https://example.com:2345/123` → `https://destination.com:2345/123`,
  **only if port 2345 actually accepts an HTTP(S) request**.
- A configured destination base path (e.g. `https://destination.com/site`) is
  prepended to non-root requests. Destination query parameters are retained.
  An explicitly configured destination port overrides the incoming port.

Root `/` requests still drop the original query string for privacy. Paths
registered for the panel, subscription formats, SPA, imported Hiddify subscriptions,
VPN-over-HTTP transports and ACME HTTP-01 validation take priority and are **not**
redirected to the decoy. HTTP requests for those paths still upgrade to HTTPS
normally on the integrated domain gateway. Non-HTTP VPN transports (TCP/UDP)
on their own dedicated ports are not intercepted or modified; arbitrary HTTP
redirects cannot be served on a port already owned by such a transport.

For a panel installed at `/`, sign in at `/?panel=1`. With a custom base path,
continue using that path. Same-origin redirects are checked to prevent loops.

### Automatic destination discovery

Click **Find the best website automatically** in **Settings → Domains and TLS**.
The finder reuses the existing SNI/REALITY candidate catalogue (including
administrator-customized domains) and recently saved verified/stable SNI results.
It samples at most 32 DNS hostnames on HTTPS/443 using the existing shared
rate-limited TLS probe, requires trusted certificates, then checks at most 16
finalists over actual HTTPS for the homepage and `/123`. Candidates are ranked
by HTTP outcome, request latency and SNI/TLS compatibility; saved stability is
a small bonus. Failed/private endpoints and non-HTTPS destinations are excluded.

The top result is staged in the domain redirect settings. Click
**Save and apply** to commit, or pick another verified candidate/manual URL.
No VPN configuration changes occur. Checks run from the panel's server, so they
cannot prove client-side reachability or special treatment by an ISP.

In **Settings → Subscription → Profile**, enable custom appearance to use the
minimal style inspired by WhiteMAX: neutral light/dark background, a subtle grid,
centered brand and rounded buttons. Edit the title, description, logo URL and accent
color, then open an existing client subscription URL in a browser. This changes
only the web view. A custom template directory retains priority.

## Reference

Implementation was compared with HiddifyPanel's `DomainType.sub_link_only`,
`DomainType.cdn`, `Domain.show_domains` and `ConfigEnum.decoy_domain` at
commit `a3a08d66b88cef7b0d4181638db97d9f92befe2c`:
https://github.com/hiddify/hiddifypanel/blob/a3a08d66b88cef7b0d4181638db97d9f92befe2c/hiddifypanel/models/domain.py

The existing 3x-ui settings table is reused; no database schema migration is needed.

Imported Hiddify clients retain their per-client legacy URLs. Configuring a new
public origin does not invalidate those existing subscriptions.
