# Manager: SNI, certificate and decoy automation

This document records the incremental design for the `manager` branch. Reuse
existing services; do **not** add a second SNI scanner or a parallel ACME server.

## Existing modules (baseline)

- `internal/web/service/sni_jobs.go` — asynchronous, rate-limited SNI scan
  jobs, mixed candidate/neighbor discovery, 5-probe stability verification,
  ranking, history.
- `internal/web/service/sni_nearby.go` — discover certificate SANs on nearby
  public IPs and verify the discovered host against the same IP.
- `internal/web/service/reality_scan.go` — TLS 1.3, h2, X25519, trusted
  certificate, public-network checks, Reality target diagnostics.
- `internal/web/runtime/remote_sni.go` and `sni_jobs.go` — scanning from
  the chosen node, not just the master.
- `internal/domainfront/manager.go` — CertMagic ACME and HTTPS domain
  frontend; `internal/web/service/domain_certificates.go` — reuse of
  existing panel, subscription and installer certificates and ACME renewal.
- `frontend/src/api/queries/useManagedSniDomains.ts` — node-local certificate
  paths; a remote node's paths must not be applied to a different node.
- `frontend/src/pages/manager/ProtocolBuilder.tsx` — protocol × node
  recipes, per-protocol SNI/Reality target and SNI discovery.

## Domain modes (design)

Keep domain purpose separate from protocol/transport:

| Mode | Meaning | Source of TLS identity |
| --- | --- | --- |
| REALITY camouflage | Connect to an independently validated external TLS endpoint; do not deploy an HTTPS site | Remote site's real certificate; no certificate copied to the VPN server |
| Direct TLS/HTTPS | Real HTTPS inbound or HTTPS proxy on the chosen node | Valid certificate whose SAN covers the public hostname, issued/imported on **that node** |
| CDN/relay | Public hostname may route through a CDN or another node | Correct TLS origin and edge certificates according to actual transport; REALITY is not proxied via CDN |
| Subscription-only | Serve subscription links without enabling unrelated proxy protocols | Subscription endpoint's own valid certificate |
| Owned decoy site | A site actually served on a node via an existing domain-front/gateway facility | Certificate for a domain owned/controlled by the administrator |

REALITY camouflage target and an owned decoy website are **not the same feature**.

## Current safe SNI integration

The builder now calls `runSNIScan(nodeId, 'reality', '', 'combined')`:
the existing scanner discovers candidate domains and neighboring IPs
on each selected node. It shows the top-ranked results and may automatically
choose a hostname only if its tested IP matches DNS (`dnsMatchesIP === true`),
TLS 1.3/h2/X25519 and certificate checks pass, and at least four repeated
probes succeed. This conservative rule prevents a neighboring certificate
hostname from being deployed against unrelated DNS. Server-side preflight
still validates the selected target before publication.

A scan from the server **does not prove** accessibility from the end user's
ISP. Never mark a protocol as definitely working on an arbitrary mobile
operator based on the server scan alone.

## Implemented in the `manager` branch

- Reuses the panel's existing node-local, combined SNI scanner and
  repeated-probe DNS/IP verification; neighbor-only certificate names are
  shown, not silently deployed as unrelated DNS destinations.
- Keeps certificates for identical hostnames on different nodes instead of
  collapsing them into one list entry.
- Reuses active Domain Front and Telemt certificates, and matching existing
  panel/subscription certificate pairs, including those exposed by authenticated
  remote nodes. Each item remains scoped to its originating node, with
  issuer/expiry shown when available.
- Xray stream-TLS and node-native NaiveProxy, AnyTLS, MASQUE, TUIC,
  VetroffProxy and TrustTunnel recipes use their own expected TLS certificate
  configuration format and node-local certificate paths. Expired certificates
  are excluded. Per-node TLS hostname overrides are supported. Missing
  certificates are skipped rather than substituting another node's files.
- The builder's explicit opt-in CertMagic action can request up to 30
  missing (node, hostname) certificates in a batch. It uses the already
  deployed Domain Front TLS manager on that node to host an owned HTTPS
  cover site, renew its certificate and supply node-local paths. Certificate
  issuance is asynchronous, so recipes must be built only once a valid
  certificate appears in the managed inventory.
- Backend preflight/rollback remains in charge of actual publication.
  The interface does not claim a server-side scan guarantees accessibility
  from the subscriber's network.

## Delivered workflow on this branch

### Shared inbound and client creation
- `POST /panel/api/manager/subscriptions/bulk-shared` creates 1–100
  independent subscriptions and queues per-client credential creation against
  selected, **existing** common inbounds. All rows/jobs are created atomically.
- Each client gets a distinct credential and subscription token while the
  common inbound listeners and ports are reused. The existing worker handles
  attachment and publishes results only after its preflight/apply checks.
- Existing manager-generated inbounds are excluded from the common pool, so
  deleting a subscription cannot delete another subscriber's shared listener.
- The manager UI has a bulk creation dialog. Shared inbounds are still created
  through the normal inbound editor; automatic pool creation from checkboxes
  is a separate operation that needs collision-safe reusable pool ownership.

### HTTPS decoy and certificate issuance
- The Domains tab offers a separate, explicit owned-website wizard:
  `POST /panel/api/manager/domains/cover-site`. It selects the main server
  or a directly managed, online node, an operator-controlled DNS name, and an
  ACME contact email with acceptance of Let's Encrypt terms.
- It reuses **the same Domain Front / CertMagic service** already in the
  panel (including renewal). The `sni_only` site serves its generated static
  cover content and does not expose administration, subscriptions, or inbounds.
- Existing domains and listeners are retained. Conflicting usages, an older
  node without the Domain Front API, missing DNS ownership or an occupied
  HTTPS listener must fail visibly, without replacing another site's config.
- This local website is not the external REALITY target. The latter is
  configured separately and requires the usual SNI scanner and REALITY tests.

### SNI fallback at generation time
- A REALITY recipe can retain up to six independently verified backup SNI
  hostnames per node. The backend validates uniqueness and DNS host syntax;
  a pinned external REALITY target cannot be mixed with these backups.
- During preflight, manual generation, and scheduled rotation the primary
  candidate is checked first and backups are tried in order if necessary.
  On complete failure the current published generation is left unchanged
  and the existing rollback process applies. The selected candidate is
  recorded in the new published entries.
- **Opt-in live health monitor**: setting `monitorSni` on a subscription
  starts a bounded `sni_health` job on the configured interval (5–1440
  minutes; 10 by default). The scheduler queues at most 12 subscriptions
  per minute and checks up to 8 published connections per job. Only
  manager-owned, already published REALITY connections with DNS-backed
  SNI reserves are eligible.
- Three independent failed scheduled checks, separated by the configured
  interval, are required. Switching is limited to once per 30 minutes
  for each connection. Candidate reserves are checked on the node hosting
  that inbound using existing bounded SNI scanner/probe capabilities.
  A scanner error is an **unknown health state**, not proof of a broken SNI.
  DNS/IP agreement, a valid certificate, TLS 1.3, h2, X25519 and at least
  4/5 successful probes are required before using a reserve.
- The live update changes only REALITY SNI/target on the existing inbound,
  preserves its port, client identities, private key and short IDs, and
  publishes the actual hostname only after the runtime accepts the change.
  On an apply/persistence error the original inbound is restored. Unit and
  integration tests cover stability, cooldown, key/credential preservation
  and a simulated failed-core rollback.
- **Availability qualification**: updating a running core can briefly
  interrupt existing connections depending on the runtime; the monitor
  does **not** promise zero packet loss or reachability from every ISP.
  Normal full-generation rotation may still rotate server keys.

## Operational limits and optional future extensions

- The two previously requested additional features (live SNI monitoring and
  per-node automated TLS issuance) are implemented with the above safeguards.
  Deployment still requires integration verification and operator-controlled
  DNS, HTTPS/ACME listener availability and compatible remote-node versions.
- ACME HTTP-01/TLS-ALPN-01 only works when the required challenge listener
  is reachable on the intended node. If a VPN already owns TCP 80/443,
  the gateway **refuses the conflict**; it does not temporarily take over an
  active listener. A DNS-01 provider integration or explicit port-sharing
  configuration is a separate choice, not an implicit feature.
- External certificates installed by another provider can be used when
  present and valid; renewal remains that provider's responsibility unless
  the administrator provisions the domain under CertMagic.
- Future optional improvements: issuer/SAN/renewal-method metadata for
  every legacy cert source, a DNS-01 provider integration for contested
  80/443 environments, and explicit IP+SNI pinning for neighboring-IP
  discoveries after independent server-side verification.
- Shared inbound creation is an administrative action in the existing
  Inbounds editor; the Manager bulk flow creates up to 100 independent
  clients against such shared listeners rather than one listener per client.

## Cover-site templates and main-node TLS

- SNI-only websites can choose one of the existing built-in/MTProxyL
  cover templates in the Manager create/update form and Domains & TLS editor.
  The catalog/template selection is saved per hostname, and rendered through
  Telemt's existing validated immutable site snapshots.
- When the site is hosted on a remote node, **Issue on main node** optionally
  registers a separate `cert_only` domain on the controlling panel.
  CertMagic uses the main node's Cloudflare/DigitalOcean DNS-01 credentials
  to issue and renew its local certificate. No remote private key is exported
  and no extra HTTP/HTTPS listener is opened on the main node.
- Configure the main panel's DNS-01 provider, zone and credential first.
  Issuance is asynchronous. Once the certificate is valid, its local
  `certFile` and `keyFile` are available in the certificate inventory and
  managed TLS/SNI listing. Paths remain node-local.
- Partial success is reported: a remote site may be created even if issuing
  the certificate on the main node fails.

## Automatic SNI-to-certificate binding

- The inbound TLS selector searches for a valid certificate matching the
  selected SNI **on the inbound node itself**. It auto-fills paths for
  supported Xray/native protocols, preserving manually entered paths.
- Where main-node TLS files are missing, the form offers opt-in DNS-01
  issuance with email and explicit ACME terms acceptance. The Manager
  builder invokes this main-node endpoint instead of creating an unwanted
  public HTTPS cover site.
- NaiveProxy has certificate/key inputs, TrustTunnel has managed SNI selection,
  and TUIC no longer invents paths based on the domain name.
- The Manager builder supports per-protocol, per-node manual cert/key paths,
  subject to preflight verification on the selected node.
- External REALITY and ShadowTLS targets do not require a local certificate.

## Exclusions and checks

- No bulk third-party cert downloads/private key extraction.
- No certificate paths shared across different nodes.
- No arbitrary unbounded internet scanning; reuse the existing bounded
  queue/limiter and SSRF protections.
- No automatic SNI replacement based only on latency or a single success.
- No use of old Hiddify Reality Scanner tooling as a prerequisite; its own
  published instructions say that the older standalone scanner is not
  maintained for HiddifyManager v13/v14.

## Protocol settings audit (26 inbound protocols)

- Audited the 26 inbound protocol schemas and their corresponding forms,
  including the shared TLS, REALITY and transport configuration sections.
- Restored editable NaiveProxy QUIC congestion control, PingTunnel upstream
  forwarding, FPTN hostname/certificate/private-key file and PEM pairs,
  metrics key and container image, and OpenFlux transport context.
- For VetroffProxy, TLS is mandatory in all transport modes; `autoTLS`
  finds a valid matching certificate, while disabling it exposes manual
  certificate and private-key paths. Selecting a managed SNI only fills a
  valid same-node pair and does not replace custom files.
- The TUIC, NaiveProxy, AnyTLS, MASQUE and Xray TLS editors likewise preserve
  imported/manual certificate paths, reject expired managed certificates,
  and treat the local node identifier `0` as the main node.
- FPTN is distinct: it needs an **RSA** server certificate for its RS256
  access tokens. The general SNI TLS picker must not silently install an
  arbitrary ECDSA certificate for FPTN; the FPTN-specific advanced form
  accepts explicit RSA certificate/key files or PEM material.
- REALITY and ShadowTLS camouflage require no local server certificate.
