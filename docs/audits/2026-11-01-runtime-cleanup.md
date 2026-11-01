# Local runtime cleanup audit

Work branch: `update`. Baseline for this change:
`d5be9d8fa0101ef2b44f02431386b0e63d249804`.

## Removed redundant code

`Local.isNative()` returned true unconditionally. Its false branches could
never run, yet still described obsolete direct API mutation and TUIC startup
paths. Remove this private helper, the unreachable branches, duplicate final
returns and the unreachable no-op batch scope. Keep the actual configuration
apply, batching, error propagation, protocol transitions and sidecar cleanup.

`AddClient` also built a twelve-field map for `AddUser`, which did not read
the map. Pass nil instead and remove the now-unused `wgKeepAlive` helper and
strconv import. Local client updates continue to regenerate from committed
database state, including renamed and disabled users. Public method signatures
and compatibility dependency fields are retained.

The README advertised Discord integration even though its UI, backend and
documentation were removed by `0210317c`. Remove that obsolete capability
claim. Discord domain/routing categories are unrelated and remain valid.

## Verification

The new dispatch test passes both before and after cleanup. It checks VLESS
and TUIC under four configured core labels (sing-box, legacy hiddify-core,
Xray and upstream sing-box): add inbound, add client, disable/rename user,
update inbound and delete inbound retain five configuration applies and do
not use the old direct API callback.

- Full runtime package: 171 test/subtest pass events with
  `go test -race -shuffle=on -json ./internal/web/runtime -count=1`.
  No skips, failures or race reports; package result passes.
- Twelve targeted service tests pass without skips. They cover committed-state
  apply, bulk client changes, no-op/rollback, attach/detach, invalid candidates,
  binary rejection without consuming stats, concurrent batch changes, canceled
  publication, watchdog behavior, previews, external outbound rollback and
  avoiding restart for unchanged configurations.
- `go vet ./internal/web/runtime` and `git diff --check` pass.
- Changed Go files formatted using gofumpt v0.12.0.

## Measurement

`BenchmarkLocalAddClient`, Go 1.27.3, Linux amd64, 200 ms sample:
416.2 to 16.14 ns/op, 648 to 0 B/op, 5 to 0 allocations/op.
The benchmark uses a no-op apply callback to isolate dispatch/map overhead;
it does not measure real core reconfiguration time.

## Remaining scope

This is the third focused improvement in the update branch, alongside the
file-tail and in-memory log-buffer optimizations. It does not complete the
14-stage audit or establish full protocol interoperability. CI results must
be checked on the final branch SHA after publication; results from the prior
two-commit snapshot do not validate this cleanup.
