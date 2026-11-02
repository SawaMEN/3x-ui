# Native subscription manager

Open **Manager** in the main menu (`/panel/manager`). This is a native Go/React implementation of the orchestration workflows reviewed in [denpiligrim/3dp-manager](https://github.com/denpiligrim/3dp-manager), revision `f06ff733836618cf80bd2bf543d3ccf36d422ad0` (GPL-3.0). It uses the panel's database, authentication, runtime and subscription renderer rather than running a second application or PostgreSQL server.

## Workflow

1. Add nodes in the Nodes tab. The existing node registry supplies API token/mTLS authentication, certificate verification, connection testing and core compatibility checks. Standalone manager username/password sessions are replaced by this panel's node authentication.
2. Create an inbound template using **Create template** in the subscription editor. All client-capable protocols offered by the fork's normal inbound form are available. Configure TLS certificates, transport and obfuscation there; paths must exist on the destination node.
3. Create a subscription, choose a panel client or leave automatic client creation selected, and add up to 20 entries. An entry can use an existing inbound, generate a new inbound from a template, or contain an external share link. Listener-only protocols without a client subscription format are not presented as client templates.
4. Choose a destination node, connection name, flag, port, optional SNI and relay for rotating entries. Existing mode preserves its inbound's node, port and SNI. `random` selects a port in 10000–59999; explicit ports are checked by the normal node-scoped conflict validator. Automatic rotation requires random ports so generations can overlap safely. An occupied fixed port is reported as a conflict, never freed by deleting another inbound.
5. **Save and generate** queues generation. Copy the subscription URL or QR code. `/bus/<UUID>` works on the panel under its configured base path and on the subscription listener. The default body is a base64 subscription; `?format=raw` returns plain links. Select sing-box, Hiddify or Xray in the format picker for JSON (`?format=singbox`, `?format=hiddify`, `?format=xray`). JSON-only protocols such as MASQUE fail with HTTP 406 in raw output rather than disappearing. A format incompatible with any selected protocol is rejected. Some combinations, such as VetroffProxy and JSON-only protocols, require separate compatible bundles. The UUID remains unchanged across edits and rotation.
6. Enable automatic rotation with an interval of at least 10 minutes. **Rotate automatic subscriptions** queues enabled subscriptions that opt into automatic rotation. Operations are processed one at a time with up to three retries for creation, attachment and cleanup.

## Protocols and metadata

The manager delegates share links to the fork's native subscription renderer, including VLESS, VMess, Trojan, Shadowsocks, Hysteria, WireGuard, AmneziaWG, MTProto, TUIC, NaiveProxy, MASQUE, Snell, AnyTLS, ShadowTLS, Mieru, Sudoku, VK Turn, TrustTunnel, FPTN, OpenFlux and VetroffProxy where the selected core/runtime supports them. This preserves protocol-specific keys and options instead of translating them through a short upstream protocol list. A generated inbound has an empty client list before the selected identity is attached through `ClientService`.

Each subscription has its own name, note, support URL, website URL and Happ/INCY profile. Routing profiles accept JSON objects, base64 or the client deep link; INCY additionally accepts an HTTP(S) profile URL. Traffic and expiry come from the selected panel client and can be managed in Clients. Expired, disabled or exhausted clients cannot download the managed bundle. Revoking access to arbitrary external servers still requires revocation on those servers.

SNI domains support text/file import, export, replacement and verified search through the existing scanner. IP literals are skipped. Selected node and relay domain names are added automatically. A random SNI must be suitable for the template: Reality must accept the target, and TLS certificates must cover the chosen name. The manager does not issue replacement certificates.

## Queue and recovery

The previous published entry set remains available while new entries are staged. Configuration is applied through `runtime.Runtime`; local generation confirms a synchronous core apply before publishing. Only after publication are older manager-owned inbounds retired. A failed generation leaves the published entry set intact. Failed staging rows retain the subscription's unique ownership tag and are cleaned up after a later successful generation or deletion.

The journal has status/process filters and 20 entries per page. It refreshes while the page is active. Finished operations are retained for 30 days, with a 50000-record retention cap. The queue survives restarts. Running operations become **interrupted**, release their active lease, and are not replayed automatically; inspect the node and start a new generation after checking its state. Queued operations resume. Duplicate requests for an active target return its existing operation.

Manager deletion only discovers inbounds with its own UUID ownership prefix. Existing templates and manually selected clients are retained. An automatically created client is deleted with its subscription. Do not rename manager-owned inbound tags or use generated inbounds as templates.

## Relay

Relay forwarding requires root SSH access, `iptables`/`ip6tables` and systemd already installed. Supply the SHA256 SSH host key fingerprint from the server console, for example `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub`. Host key mismatches fail closed. Passwords/private keys are write-only API inputs; their at-rest encryption follows the panel's `NODE_TOKEN_ENCRYPTION` configuration.

Forwarding uses separate owned DNAT/SNAT/filter chains and only the actual ports of published/staged entries, over TCP and UDP. Rules are persisted in a dedicated systemd service. Installation does not install packages, enable UFW or flush unrelated chains. Root forwarding is enabled for the selected IP family. Remove a relay from entries and regenerate before uninstalling it. The manager removes only its own service, scripts, hooks and chains.

## Validation boundaries

Automated tests cover rotation and stable identity, failure-before-publication, runtime rejection, ownership boundaries, queue deduplication and recovery, domain/profile validation, native protocol bundle rendering, quota/expiry checks, SSH secret redaction, generated relay scripts and editor save behavior. Live SSH forwarding, actual remote VPN handshakes and PostgreSQL require their respective infrastructure; unit success alone does not establish those paths.
