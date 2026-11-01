# SNI discovery, stability and saved measurements

Open the SNI/REALITY scanner in an inbound form. It supports:

- **Candidate list / custom targets**: the configured domain list (800 built-in
  seeds by default) or supplied domains, IPs and CIDRs.
- **Find SNI near the server**: up to 256 neighboring public IPs, alternating
  below/above a reference. Leave it empty for the execution server's public IP,
  or supply one server IP/domain[:port]. A domain uses its first public IPv4,
  falling back to IPv6. The origin itself is excluded.
- **Neighbors + configured domain candidates**: discover around the reference,
  then check the execution server’s configured domain list and rank the combined results.
- **Saved results**: select a previous scan and recheck its saved IP/SNI pairs.

For node-assigned inbounds, jobs, public-IP detection, measurements, history and
cancellation execute on that selected node through the existing authenticated
node transport (including its mTLS/egress configuration). The execution location
is shown in the scanner. Upgrade both the master and node to this version.
Unsupported nodes report an error; they never silently scan from the master.
Changing the inbound's node cancels the previous scan and clears its results.

## Discovery and ranking

IP discovery extracts leaf certificate names without SNI. A trusted chain is
required before up to eight distinct non-wildcard Common Name/SAN candidates
are tried with SNI against the same IP. Only actually verified names are offered.
The combined/candidate scan also pins successful results to the verified IP,
while retaining their SNI. Multiple verified names on the same IP stay distinct.

Results appear incrementally, with discovery and stability progress. **Stop scan**
keeps already displayed results while canceling queued/active work. Closing the
modal, changing mode, restarting a search or changing nodes cancels old work.

The ten best compatible candidates each receive five further TCP/TLS connections.
Ranking prefers compatible, stable results (at least 4/5 successful connections),
then success count, median connection time and spread. Spread means max minus min
of successful measurements. Even-sized samples use the average of the middle
pair. Certificate metadata is refreshed from successful verification probes;
ML-DSA-65 eligibility uses the smallest certificate chain observed in those
probes. Failed checks do not reuse old timing or HTTP/2/DNS diagnostics. Unstable measured candidates cannot be selected. Rows that have not had the
five-probe check are explicitly marked **Preliminary** and are not recommended.

The table separates SNI/IP, TCP/TLS timing, success count, measurement date and
candidate source. A tooltip explains successful attempts, median and spread.
The final stability probe also sends a bounded HTTP/2 HEAD request over the same
already-verified TLS connection (no redirects, no additional DNS dial). Any valid
HTTP/2 response counts as protocol support; a particular HTTP status is not
required. This is an extra diagnostic, not a blanket requirement for every
protocol. DNS diagnostics report whether the hostname currently includes the
verified IP, differs, or could not be resolved.

The selected REALITY target uses the verified IP and exactly the tested SNI, and
resets `xver` to zero because the scanner did not send a PROXY header. Incompatible
REALITY endpoints and insufficient ML-DSA-65 certificate chains cannot be selected.
For domain-based protocols, DNS can route a name to a different endpoint: the
DNS diagnostic makes that distinction visible. Ordinary TLS inbounds still need
their own certificate and DNS; choosing a third-party SNI does not install one.

## Resource bounds and persistence

One job executes per panel, with at most four accepted running/queued jobs.
Canceling a queued job immediately removes it and frees its admission slot.
Every scan (including the legacy synchronous API) shares at most 32 active probes
and a 16-probes/second limiter with an eight-probe burst. Discovery, name retries,
finalist count, request size and saved-target count are bounded. SSRF protection
blocks private, loopback and link-local probe destinations, including rechecks.

Executing jobs have a six-minute deadline. Discovery reserves time for the
five-probe checks: up to four minutes for a single discovery mode, or two
minutes each for neighbor and candidate discovery in combined mode. A phase
timeout preserves verified partial results and adds a warning; it does not
prevent the remaining discovery phase or stability verification. Explicit user
cancellation still aborts the job. Polling renews a 30-second browser
lease, checked every five seconds; lost browsers cannot leave indefinite scans.
Completed snapshots expire after 15 minutes without access, with an overall
64-snapshot bound. When that bound is reached, the oldest completed snapshot
is evicted so history retention cannot block new scans. Active TCP/TLS probes
honor cancellation contexts. Snapshot/history responses are marked `no-store`.

The execution panel's database stores the latest 20 completed searches, up to
20 certificate-verified IP/SNI targets each. History is independent of the
browser and survives a panel restart. Job state is not persisted across restarts.
Saved results show their original date and should be rechecked before applying.
A history write failure is reported in the final snapshot; it is never silently
presented as a successful save.

## API

Authenticated endpoints under `/panel/api/server`:

| Method | Path | Behavior |
|---|---|---|
| POST | `/sni/jobs` | JSON `SNIScanRequest`: mode, targets, purpose, optional nodeId/recheck |
| GET | `/sni/jobs/:id` | Incremental `SNIJobSnapshot`, renews browser lease |
| DELETE | `/sni/jobs/:id` | Cancel work |
| GET | `/sni/history` | Stored completed scans |

For GET/DELETE, pass the same optional `nodeId` query used at creation. Zero or
omitted means local. Node-sync credentials can operate only on their own local
scanner; they cannot relay requests through another node. Creation accepts up to
128 KiB. `recheck` accepts at most 20 entries with IP, hostname and valid port.
The API documentation UI and generated TypeScript/Zod/JSON schemas cover these
contracts and the optional diagnostic fields on `RealityScanResult`.

The original `POST /scanRealityTargets` API remains compatible:
`mode=candidates` (or omitted) scans its supplied/configured targets;
`mode=nearby` scans around one reference. It returns `RealityScanResult[]` and
uses the same shared connection/rate limits. New UI workflows use queued jobs.

## RealiTLScanner reference and practical limits

Reference: https://github.com/XTLS/RealiTLScanner at
`e5e114747aa8c6dfa4b394a0782f241bf72bdce6` (`utils.go`, `scanner.go`, `geo.go`).
The upstream alternates neighboring addresses indefinitely for a single IP/domain,
extracts TLS certificate names and reports TLS 1.3/h2 candidates. Its optional
GeoIP database reports country, not geographic distance. This integration uses
that approach with the panel's existing TLS probe, finite bounds, certificate
verification, cancellation, ranking, persistence and node dispatch; no extra
scanner binary or GeoIP database is required.

IP proximity does not guarantee geographic proximity. Measurements are TCP/TLS
setup times from the execution server, not ICMP ping or proof of reachability
from client networks. Only bounded candidates are examined, and default
certificates containing only wildcard patterns may reveal no usable hostname.
Scanning may be flagged by the hosting provider; the shared limits constrain
traffic without guaranteeing provider acceptance.
