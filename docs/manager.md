# Native subscription manager

Open **Manager** in the main menu (`/panel/manager`). This is a native Go/React implementation of the orchestration workflows reviewed in [denpiligrim/3dp-manager](https://github.com/denpiligrim/3dp-manager), revision `f06ff733836618cf80bd2bf543d3ccf36d422ad0` (GPL-3.0). It uses the panel's database, authentication, runtime and subscription renderer rather than running a second application or PostgreSQL server.

## Workflow

1. Add nodes in the Nodes tab. The existing node registry supplies API token/mTLS authentication, certificate verification, connection testing and core compatibility checks. Standalone manager username/password sessions are replaced by this panel's node authentication.
2. Choose **Automatic generation** and a protocol/connection preset. It creates a new inbound directly on this panel or the selected node; a saved inbound template is not required. **Connection settings** opens the native protocol editor in recipe mode: it edits this subscription entry without creating a listener. Configure TLS certificates, transport and obfuscation there; certificate paths must exist on the destination node. Existing template cloning remains available as a separate mode.
3. Create a subscription, choose a panel client or leave automatic client creation selected, and add up to 20 entries. An entry can generate directly from its protocol recipe, clone a saved template, use an existing inbound, or contain an external share link. Listener-only protocols without a client subscription format are not presented as client templates.
4. Choose a destination node, connection name, flag, port, optional SNI and relay for rotating entries. Existing mode preserves its inbound's node, port and SNI. `random` selects a port in 10000–59999; explicit ports are checked by the normal node-scoped conflict validator. Automatic rotation requires random ports so generations can overlap safely. An occupied fixed port is reported as a conflict, never freed by deleting another inbound.
5. **Save and generate** queues generation. Copy the subscription URL or QR code. `/bus/<UUID>` works on the panel under its configured base path and on the subscription listener. The default body is a base64 subscription for URI-capable protocols, but switches automatically to native sing-box JSON when a published entry uses MASQUE, preserving all other compatible connections in the same subscription. Hiddify User-Agents receive one combined native document. `?format=all` follows that automatic selection; `?format=raw` explicitly requests plain links. Select sing-box, Hiddify or Xray in the format picker for JSON (`?format=singbox`, `?format=hiddify`, `?format=xray`). JSON-only protocols such as MASQUE fail with HTTP 406 in explicitly requested raw output rather than disappearing. A format incompatible with any selected protocol is rejected. Some combinations, such as VetroffProxy and JSON-only protocols, require separate compatible bundles. The UUID remains unchanged across edits and rotation.
6. Enable automatic rotation with an interval of at least 10 minutes. **Rotate automatic subscriptions** queues enabled subscriptions that opt into automatic rotation. Up to four workers process operations concurrently; operations sharing a destination core or relay run sequentially. Creation, attachment and cleanup use bounded retries.

## Protocols and metadata

The manager delegates share links to the fork's native subscription renderer, including VLESS, VMess, Trojan, Shadowsocks, Hysteria, WireGuard, AmneziaWG, MTProto, TUIC, NaiveProxy, MASQUE, Snell, AnyTLS, ShadowTLS, Mieru, Sudoku, VK Turn, TrustTunnel, FPTN, OpenFlux and VetroffProxy where the selected core/runtime supports them. This preserves protocol-specific keys and options instead of translating them through a short upstream protocol list. A generated inbound has an empty client list before the selected identity is attached through `ClientService`.

Each subscription has its own name, note, support URL, website URL and Happ/INCY profile. Routing profiles accept JSON objects, base64 or the client deep link; INCY additionally accepts an HTTP(S) profile URL. Traffic and expiry come from the selected panel client and can be managed in Clients. Expired, disabled or exhausted clients cannot download the managed bundle. Revoking access to arbitrary external servers still requires revocation on those servers.

SNI domains support text/file import, export, replacement and verified search through the existing scanner. IP literals are skipped. Node and relay API addresses are not added to the Reality pool automatically. Random SNI selection applies to Reality; the destination checks the target. Certificate-backed TLS retains its configured domain, which its certificate must cover. The manager does not issue replacement certificates.

## Queue and recovery

The previous published entry set remains available while new entries are staged. Configuration is applied through `runtime.Runtime`; both local generation and each affected node confirm a synchronous core apply before publishing. Server-side Reality/WireGuard/AmneziaWG keys and Shadowsocks server secrets are regenerated for each new inbound; subscription/client identity is retained. After publication, previous generations remain available for the configured grace period (120 minutes by default), then maintenance removes their manager-owned inbounds. A failed generation leaves the published entry set intact and rolls back its staged listeners. Failed cleanup remains visible as `cleanup_pending` and retries cleanup without creating another generation.

The journal has status/process filters and 20 entries per page. It refreshes while the page is active. Finished operations are retained for 30 days, with a 50000-record retention cap. The queue survives restarts. Running operations become **interrupted**, release their active lease and are reconciled by cleanup without replaying generation. Queued operations resume. Duplicate requests for an active target return its existing operation.

Manager deletion only discovers inbounds with its own UUID ownership prefix. Existing templates and manually selected clients are retained. An automatically created client is deleted with its subscription. Do not rename manager-owned inbound tags or use generated inbounds as templates.

## Relay

Relay forwarding requires root SSH access, `iptables`/`ip6tables` and systemd already installed. Supply the SHA256 SSH host key fingerprint from the server console, for example `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub`. Host key mismatches fail closed. Passwords/private keys are write-only API inputs; their at-rest encryption follows the panel's `NODE_TOKEN_ENCRYPTION` configuration.

Forwarding uses separate owned DNAT/SNAT/filter chains and only the actual ports of published/staged entries, over TCP and UDP. Rules are persisted in a dedicated systemd service. Installation does not install packages, enable UFW or flush unrelated chains. Root forwarding is enabled for the selected IP family. Remove a relay from entries and regenerate before uninstalling it. The manager removes only its own service, scripts, hooks and chains.

## Validation boundaries

Regression tests cover generation without saved templates on both destinations, stable subscription identity, independent protocol settings, regenerated server keys, runtime rejection before publication, grace periods, rollback and interrupted cleanup, owner boundaries, device limits, quota/expiry, compact pagination, readiness and encrypted recovery. Frontend tests check recipe submission without creating listeners from the editor. The preflight port checks share the panel's protocol-aware conflict logic, including TCP/UDP coexistence and additional QUIC listeners.

`tools/ci/manager-smoke.py` runs two isolated real panel/core installations. It imports a mixed local VLESS and remote VMess/WebSocket subscription into the owner core, checks echo payloads, rotates, verifies grace-period listeners, tests node failure/recovery and checks encrypted restore. Its isolated routing template permits the private bridge echo server; production private-egress protection remains unchanged.

The CI workflow also includes frontend checks, Go tests and race checks, PostgreSQL, code generation, vulnerability scanning and native component integration. A test definition alone does not establish a successful run; use the GitHub Actions result for the tested commit. Live SSH forwarding, every protocol/transport combination, operator-specific reachability and browser visual QA require separate verification. HWID admission controls subscription retrieval and does not prevent copying an exported protocol credential.

## Reliability and complete recovery

See [Manager reliability and recovery](manager-hardening.md) for HWID enforcement, generation rollback, transition periods, readiness checks, compact listing and the encrypted recovery CLI.
