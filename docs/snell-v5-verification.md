# Snell v5 QUIC Proxy verification — 2026-10-25

## Implementation

The pinned core implements Snell v5 TCP, UDP-over-TCP and optional UDP-over-UDP
QUIC Proxy. Both native UDP entry points (`ListenPacket` and `DialContext`)
select the QUIC path only when the first datagram is a QUIC v1/v2 Initial.
Other datagrams retain UDP-over-TCP. The inbound listens on TCP and UDP on the
same port when `quicProxy` is enabled; the panel reserves both transports when
checking listener conflicts. The option remains off by default.

The previous implementation used Argon2id with 8192 KiB memory. The wire format
requires 8 KiB (time=3, lanes=1, output=32 bytes, AES key=first 16 bytes).
Self-round-trip codec tests did not detect that error because both ends used
the same wrong value. The corrected value passes an independent Surge capture
and real exchanges with the official v5 server.

Other corrections:

- Retransmitted Initials retain an authenticated envelope until the first reply.
  Existing server flows unwrap matching retransmissions instead of passing Snell
  ciphertext to the QUIC endpoint. A different destination/user is rejected.
- A full UDP input queue drops the datagram instead of destroying the session.
- Read/write deadlines, Close during startup, cancellation of pending dials and
  idle flow cleanup are implemented. Dial-context cancellation after handshake
  no longer closes an established UDP association.
- User identity and decoded traffic pass through the native packet router.
  A PSK-only client is attributable only to a sole configured user; multiple
  users require an individual userkey.
- Version 6 rejects the version-5-only QUIC option.

## Evidence

The tests live in `protocol/snell/quic_proxy_integration_test.go` and
`quic_proxy_test.go` in the pinned core fork.

- HTTP/3 request/response traffic through native inbound/outbound for QUIC v1
  and v2, using both UDP API entry points.
- Official `snell-server v5.0.1` accepts our client and exchanges real HTTP/3
  v1/v2 payloads. The test binary is downloaded from
  <https://dl.nssurge.com/snell/snell-server-v5.0.1-linux-amd64.zip> and verified
  against SHA-256
  `9bea1c2b9e35b73b31634856c04d18c393072b9e5dcde6a32781d8b8f908c539`.
- A public real-Surge HTTP/3 Initial capture decodes to the independently known
  target and payload. Provenance and GPL-3.0-or-later licensing are recorded in
  the core's `protocol/snell/testdata/README.md`.
- First outgoing Initial and first incoming response are deliberately dropped;
  HTTP/3 recovers without creating another authenticated flow.
- Three minutes of sustained HTTP/3 v1/v2 echo traffic pass with the race
  detector (185.276 seconds including the other v5 checks).
- Ordinary UDP falls back to TCP and completes 100 echo exchanges.
- User-routing metadata, bidirectional byte counters, cross-user injection
  rejection, retransmitted Initial handling, deadlines, Close during a pending dial,
  queue overflow, expiry and authenticated reconnection are covered.

CI performs the pinned-official-server test, race detection, the sustained v5
traffic test and the existing v6 idle/reconnect regression checks:

```sh
SNELL_V5_SERVER=/path/to/snell-server \
SNELL_V5_SOAK_TEST=3m SNELL_V6_IDLE_TEST=130s \
go test -race ./protocol/snell ./option -count=1 -timeout 10m
```

## Limits

The Surge application itself and an actual ISP/DPI path were not run. A recorded
Surge packet is evidence for decoding that wire format, not complete live-client
certification. Three minutes of traffic does not establish hours-long production
reliability or production quota/database behavior.

The protocol maps an authenticated source tuple to an endpoint. An authenticated
Initial can establish a fresh mapping after expiry or a source-port change;
opaque short-header traffic alone cannot authenticate a new NAT binding. No
transparent migration across arbitrary NAT changes is claimed.

Panel regression tests (Snell, runtime identity and port conflicts) and panel
compilation pass. The protocol tests also pass in the full parent runtime with
all panel build tags and overlays (1.360 seconds). The release build script runs
both v5 and v6 protocol checks before producing a binary.

If an older experimental native v5 client used the incorrect 8192-KiB format,
update its core along with the server. Existing PSKs/userkeys are retained;
ordinary TCP/UDP-over-TCP and v6 are unaffected by this QUIC wire-format fix.

Full linux/amd64 runtime compilation and all overlay tests passed using Go
1.26.9. The resulting binary accepted v5 QUIC configuration and explicitly
rejected the same option for v6. Bundled Cronet validation and build identity
checks also passed. The core's CI completed successfully with the pinned
external v5 server, sustained HTTP/3 and v6 idle checks enabled.
