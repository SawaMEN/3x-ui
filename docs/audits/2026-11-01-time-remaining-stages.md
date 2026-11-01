# Follow-up audit: remaining 3x-ui stages on `time`

Date: 2026-11-01. Branch: `time`. Draft PR: #278. Do not merge
into `main` until both full CI and the binary-backed release build pass.
Baseline is the merged audit #277, `main` at
`4aa744111b4649be719e18a48166a20debd85038`. Comparison begins
at `v3.9.8` (`416a66a3343d446b9b18a2a8b2b0eb8771ea72fa`);
the recorded `dev-latest` tag is `8b59a974c23ae4c7fb9072d46fa18abb5390761a`.
There is no persistent `dev` branch at the recorded baseline.

## Confirmed defects and fixes on time

1. **Release blocker — unsupported legacy TLS suites.** The pinned core
   rejected `TLS_RSA_WITH_AES_128_GCM_SHA256` during actual AMD64 binary
   constructor checks, despite successful Go-only tests. Reject insecure
   or unknown suites at both translated and native TLS serialization
   boundaries, with an actionable error; leave saved settings unchanged
   instead of quietly rewriting a security policy. Use supported
   ECDHE suites in positive binary fixtures and assert early rejection
   for an explicitly configured RSA key-exchange suite.
2. **Cross-database backup safety.** `ExportPostgresToSQLite` deleted
   the previous output before connecting to its PostgreSQL source.
   It now writes to a private temporary SQLite file next to the
   destination and renames it only after copying and closing succeed.
   A regression test verifies that a source connection failure leaves
   the previous destination bytes and no temporary files.
3. **Multi-node egress callback deadlock.** `Manager.NodeEgressProxyURL`
   previously called an external resolver while holding `Manager.mu`.
   The resolver could re-enter `SetNodeEgressResolver` and deadlock.
   Copy the resolver under a read lock, release the lock before invoking,
   and test reentrancy plus 4,000 concurrent replacement/lookup iterations.

## Stage-by-stage evidence and sign-off boundaries

1. **Commit inventory — IN PROGRESS.** Prior audit records 1,223 commits
   from 3.9.8 to the main snapshot and the last 131 changes after the
   moving DEV tag. Commit-by-commit ownership classification and test
   mapping are not completely signed off.
2. **Build, CI and release — IN PROGRESS.** The merged baseline passed all
   20 standard CI jobs, but its AMD64 release build failed. This PR
   enables release builds on pull requests and validates the real pinned
   core. Sign off only with green CI, CodeQL, Docs CI, both release
   architectures and published artifact checksum checks.
3. **Database migration/recovery — IN PROGRESS.** Previously fixed
   duplicate migration models and added row-copy regression tests.
   This PR adds atomic PostgreSQL-to-SQLite backup publication.
   Still requires representative v3.9.8 SQLite-to-PostgreSQL upgrades,
   reverse export, rollback and interrupted-import acceptance.
4. **Core runtime/lifecycle — IN PROGRESS.** Existing Go lifecycle tests
   cover core selection, native configuration, process identity,
   candidate publication and rollback. This PR adds manager callback
   deadlock tests; real long-running core restarts and stop/restart
   races across platforms remain to be exercised.
5. **Inbound/outbound protocols — IN PROGRESS.** Release PR jobs run
   `TestPinnedBinaryChecksGeneratedConfigurations` and additionally
   `TestPinnedProtocolTransportMatrix` (54 combinations of VLESS,
   VMess and Trojan with nine transport variants, both directions).
   These are core configuration/constructor checks, **not** proof
   of 54 successful TCP/UDP data transfers. Native interoperability
   still needs external compatible client processes and UDP/TCP E2E.
6. **Clients/traffic/limits/subscriptions — IN PROGRESS.** The standard
   CI runs subscription output tests, live TCP/UDP client rate tests,
   plus Mieru, Sudoku and MASQUE budget tests. Real multiple-device,
   monthly renewal and 24-hour accounting reconciliation remain.
7. **Routing/DNS/gateway/firewall — IN PROGRESS.** Standard CI checks
   MEKO rules in a Linux network namespace and runs routing unit tests.
   PR release adds `TestGeoRoutingModifiedSingBoxCheck` using the pinned
   binary. DNS leak tests across real network namespaces and failure
   recovery remain.
8. **Telemt/Vetroff/sidecars — IN PROGRESS.** Standard CI tests official
   Telemt on ARM64 and AMD64, WEB frontends, sidecar egress and
   Vetroff-related Go tests. Running a separately provisioned Vetroff
   server and client end-to-end is still required.
9. **API, auth, security and multi-node — IN PROGRESS.** Baseline CI
   includes CodeQL, govulncheck and API authorization tests. This PR
   fixes an egress-resolver lock inversion; further adversarial
   mTLS, remote egress SSRF and multi-node isolation tests remain.
10. **Frontend/mobile/WebSockets — IN PROGRESS.** Standard CI runs
    formatting, lint, typecheck, browser/component tests, Storybook
    and build. Real mobile viewport, session expiry, reconnection and
    browser-device E2E still need separate acceptance.
11. **Installer/updater/rollback — IN PROGRESS.** Standard CI runs shell
    installer/rollback tests and Arch package validation. This PR
    removes an export failure that could destroy an earlier backup.
    Clean-host installs, interrupted updates and full OS rollback
    matrices remain.
12. **Race/load/soak — IN PROGRESS.** Standard CI includes broad race
    detection and service/subscription shards. Added 4,000 concurrent
    manager egress operations; do not confuse this with a 24-hour soak
    test or sustained production throughput benchmarking.
13. **Last 131 main commits after DEV tag — IN PROGRESS.** The earlier
    audit recorded 57 changed paths; latest fixes cover high-risk
    diagnostics, native TLS config and database migration, but a full
    behavioral review of every changed path is not signed off.
14. **Final independent regression — NOT STARTED.** Requires the final
    locked SHA, full successful jobs, release binary checks, manual
    compatibility evidence and no unresolved high-severity findings.

## Reproduction and limits

- General: `go test ./... -count=1` and the complete CI workflow.
- Concurrency: `go test -race ./internal/web/runtime -count=5`.
- PostgreSQL: the CI `postgres-durable-first` job plus a populated
  v3.9.8 database fixture round-trip.
- Actual core: set `HIDDIFY_CONFIG_CHECK_BINARY` to the pinned
  executable and run
  `go test ./internal/hiddify -run 'TestPinned(BinaryChecksGeneratedConfigurations|ProtocolTransportMatrix)' -count=1`.
- Protocol-level coverage above checks generated configurations, not
  end-to-end network availability during internet restrictions.
- Results are attached to individual GitHub Actions SHAs. Do not
  treat a successful run for an older SHA as certification of new
  changes; no manual E2E has been claimed by this report.
