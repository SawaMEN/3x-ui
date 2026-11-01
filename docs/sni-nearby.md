# Nearby SNI discovery

In an inbound's SNI/REALITY scanner, choose **Find SNI near the server** and
press **Scan**. Leave the reference empty to use the panel server's public IP,
or enter the intended VPN server IP/domain (optionally with a port). The latter
is useful when the panel and VPN endpoint run on different machines. A domain
with multiple public addresses uses the first IPv4, falling back to IPv6.

The search checks up to 256 public IPs, alternating below and above the reference
address, with 32 concurrent probes. It excludes the reference itself and blocks
private/loopback/link-local destinations using the panel's existing SSRF guard.
There is no infinite scan and no additional binary or GeoIP database to install.

For each IP:

1. Connect without SNI and extract a usable domain from the leaf certificate.
2. Check the certificate chain and repeat the handshake with the discovered SNI
   against that same IP, validating the hostname and trust chain.
3. Return only SNI-verified results. Preserve the scanned IP as the REALITY
   target and the verified hostname as SNI; include TLS, ALPN, key exchange,
   certificate and connection-time details from the final probe.

REALITY results rank compatible TLS 1.3 / h2 / X25519 endpoints first, then by
measured connection time from the panel server. Other protocol forms retain
their existing compatibility checks. The measurement includes TCP and TLS setup;
it is not ICMP ping, a geographic distance, or a client-network reachability test.
Neighboring IPs may belong to different networks/locations. Only 256 neighbors
are examined, so an empty result is possible. Scan activity may be flagged by
the hosting provider.

For ordinary TLS inbounds, selecting a third-party SNI does not install a
certificate: the operator still needs a certificate and DNS for their own server.

## RealiTLScanner analysis

Reference: https://github.com/XTLS/RealiTLScanner at
`e5e114747aa8c6dfa4b394a0782f241bf72bdce6` (`utils.go`, `scanner.go`, `geo.go`).

RealiTLScanner alternates neighboring addresses indefinitely when given a single
IP/domain, probes TLS without SNI for IPs, extracts certificate information, and
reports TLS 1.3 / h2 candidates. Its optional GeoIP database reports country;
it does not calculate geographic distance or automatically find the closest SNI.

This integration implements that search approach using the panel's existing
TLS probe rather than copying the standalone program. It adds finite bounds,
request cancellation between probes, private-network protection, certificate
trust checks, an actual SNI verification handshake, and UI selection. Active
probes finish within their existing timeout after request cancellation.

## API

`POST /panel/api/server/scanRealityTargets` (existing authenticated endpoint):

- `mode=candidates` (or omitted): existing list/domain/IP/CIDR scan.
- `mode=nearby`: neighbor discovery; `targets` is one server IP/domain[:port],
  or empty for automatic public-IP discovery. Invalid modes and private
  references are rejected. Returns the existing `RealityScanResult[]` format.

The default candidate list and existing scan behavior are preserved.
