# Snell v6 verification — 2026-10-25

Scope: panel PR #224 and hiddify-sing-box PR #3. Snell v6 uses the
pinned sing-snell `bc5a12ac736f235b2de2926ecd2791cc925e6b8c` implementation.

## Corrections

- Preserve the user key when the email's case or surrounding whitespace changes
  and the password is omitted from an edit. Previously this generated a new key
  and broke existing subscriptions.
- Validate native outbound user keys against the core's 255-byte limit.
- Validate UTF-8 key lengths, duplicate enabled names/keys and control characters
  in the frontend before submission. Empty keys are generated on save.
- Always populate the authenticated routing user, including the numeric fallback
  for unnamed users. Reject invalid authentication indexes before indexing.
- Synchronize the runtime version constant with the build manifest. The earlier
  QUIC PR changed the manifest alone and failed the mandatory source audit.
- Correct the ray2sing lock to the actual gitlink of the pinned parent core,
  `8757f68feff645ab63b87bddcaba88a11fba9aef`.
- Reconcile the temporary build module graph after applying pinned overlays.
  Without this, Go stopped the runtime build with `updates to go.mod needed`.
- Add a regression test matching runtime identity to the build manifest.

## Protocol checks

Core tests are in `protocol/snell/v6_integration_test.go`:

- 120 loopback TCP sessions for each default/unshaped × reuse off/on combination
  (480 sessions); varied payload sizes, logical EOF and client pool reset.
- 100 UDP-over-TCP exchanges per mode (200 datagrams); payload/address integrity
  and authenticated routing identity.
- Rejection of invalid/missing user keys and wrong deployment PSK.
- Native outbound TCP, UDP DialContext and ListenPacket in both modes.
- Recovery of open TCP and UDP sessions after 130 seconds of actual idle time,
  in all four mode/reuse combinations.

The workflow enables the race detector and idle checks:

```sh
SNELL_V6_IDLE_TEST=130s go test -race ./protocol/snell ./option -count=1 -timeout 5m
```

Use the deployment-pinned Go 1.26.9. A diagnostic run with Go 1.27.x failed in
Psiphon TLS initialization with a ConnectionState layout mismatch outside Snell.

## Limits

These are loopback integration tests of the pinned wire implementation and
native adapters. They do not certify Surge compatibility, production quota
handling, DPI, packet loss, NAT changes or long-lived internet sessions. No
production server access was supplied. The reported one-minute operation /
two-minute timeout cycle was not reproduced; the credential fix must not be
claimed to establish its cause.

Snell v6 does not use v5 QUIC Proxy. UDP remains encrypted over TCP. Exported
subscriptions leave reuse disabled by default. Client and server modes must
match.

## Current validation

- Panel tests for hiddify, snell, singbox and web/service: passed; the full
  service package took 235.993 seconds.
- Frontend TypeScript, production build and seven focused Snell tests: passed.
  The focused tests used a Node-only config to avoid unrelated Storybook
  network-interface discovery restrictions.
- Panel `go build ./...`: passed.
- Protocol and option tests with the race detector and 130-second idle test:
  passed (Snell: 133.001 seconds).
- Native Snell tests in the full parent runtime with all panel build tags:
  passed (1.032 seconds).
- Full linux/amd64 runtime build, overlay tests, version/commit identity, basic
  config validation and bundled Cronet validation: passed. The only local
  build-script substitution fetched the unpublished core commit from its local
  repository; production patches and build flags were unchanged.
- The built binary accepted native v6 inbound/outbound configs for both modes.
- Whole-process HTTP-over-SOCKS/Snell smoke test: blocked at service startup
  with `create netlink socket: operation not permitted`. It did not reach the
  Snell handshake, so it establishes neither failure nor success of that path.

The fixes are published through the existing PR branches. The published core
commit has the same verified Git tree `408721e5b83c69f8b1479548f60521cc443817c5`;
only Git commit metadata differs from the local test checkpoint.

Snell v5 QUIC Proxy remains experimental and disabled by default. Codec unit
tests are not evidence of actual HTTP/3 or Surge interoperability.
