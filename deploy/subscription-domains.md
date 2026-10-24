# Separate subscription domains and CDN

For the integrated automated gateway with Direct/CDN/Relay/subscription-only roles
and certificate lifetime monitoring, use **Settings → Domains and TLS**; see
[domains-and-tls.md](domains-and-tls.md). The options below remain available for
deployments that use their own external reverse proxy.

Configure **Settings → Subscription → Panel settings**:

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

## Domain root redirect and appearance

In **Settings → General**, enable the domain root redirect and choose a destination
or enter your own HTTP/HTTPS URL. Only GET/HEAD `/` requests are redirected (302),
without forwarding the original query string. Other paths retain their behavior.
For a panel installed at `/`, sign in at `/?panel=1`. With a custom base path,
continue using that base path. Same-origin destinations are ignored to avoid loops.

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
