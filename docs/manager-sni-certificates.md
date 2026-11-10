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

## Next steps

1. **Node-aware certificate inventory**: show certificate SAN, expiry,
   issuance method and renewal status; distinguish panel-local vs remote-node
   paths. Consume existing managed domain/certificate APIs rather than
   storing private keys in the SNI catalog.
2. **Certificate-backed recipes**: for TLS transports, require a verified
   certificate that covers SNI on the selected node. Offer existing certificate
   reuse, existing CertMagic-managed issuance (where DNS and HTTP-01/TLS-ALPN
   challenges can reach the correct node), or explicit manual paths.
   Preflight each proposed listener before mutating resources.
3. **Verified SNI/target pairs**: allow a pinned public IP:port plus matching
   SNI for neighbor-only discoveries, but only after server-side preflight
   supports independently verifying the same IP+SNI pair. Refuse loopback,
   private, reserved or otherwise prohibited destinations. Never silently
   switch a pinned target to DNS-based hostname resolution.
4. **Optional health watcher**: revalidate on a per-node schedule, keep a
   reserve pool and require repeated failed checks before proposing a switch;
   use hysteresis to prevent flapping. Use atomic staged publication with
   rollback and retain working inbounds on failure. Never rotate REALITY
   private keys or short IDs merely because its external website changed.
   Changes to subscriptions should be explicit/observable.
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
