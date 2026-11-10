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
- Xray stream-TLS recipes automatically populate SNI and file-backed
  certificate references for each selected node; expired entries are not
  selected. Per-node TLS domain overrides are available. If no certificate
  is known, that node/protocol pair is skipped by default. An explicit
  manual-only draft option still requires normal preflight.
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
- This is **not** an always-on health watcher or an in-place change of a live
  connection. A scheduled rotation can use the fallback, but REALITY keys
  still rotate as part of the normal generation workflow. In-place
  key-preserving automatic failover requires a separate verified and
  idempotent update transaction, and must not be claimed as implemented.

## Next steps

1. **Finish node-aware certificate health**: the implemented inventory
   already shows known hostnames, expiry and issuer. Extend it to report
   renewal/issuance method and complete SAN coverage for every node.
2. **Extend certificate-backed recipes**: Xray stream TLS now reuses known
   certificate files on their owning node. Add carefully validated per-node
   CertMagic issuance and native-protocol certificate mappings, using the
   existing certificate manager rather than a parallel ACME implementation.
3. **Verified SNI/target pairs**: allow a pinned public IP:port plus matching
   SNI for neighbor-only discoveries, but only after server-side preflight
   supports independently verifying the same IP+SNI pair. Refuse loopback,
   private, reserved or otherwise prohibited destinations. Never silently
   switch a pinned target to DNS-based hostname resolution.
4. **Optional live health watcher**: the current implementation only
   uses reserves during generation/preflight. Add scheduled repeated failure
   checks and in-place, key-preserving, transactional live failover with
   hysteresis before enabling uninterrupted automatic switching.
5. **Owned website/decoy mode**: integrate the existing domain-front manager
   to serve an optional real HTTPS page on a selected node, with validation
   of port and certificate ownership. Do not treat this as the REALITY
   fallback target.
6. **Multi-user shared inbounds**: add per-client credentials to selected
   shared inbounds, preserving existing clients/usage and isolating node
   failures. Maintain the current manager rotation/cleanup rollback.

## Exclusions and checks

- No bulk third-party cert downloads/private key extraction.
- No certificate paths shared across different nodes.
- No arbitrary unbounded internet scanning; reuse the existing bounded
  queue/limiter and SSRF protections.
- No automatic SNI replacement based only on latency or a single success.
- No use of old Hiddify Reality Scanner tooling as a prerequisite; its own
  published instructions say that the older standalone scanner is not
  maintained for HiddifyManager v13/v14.
