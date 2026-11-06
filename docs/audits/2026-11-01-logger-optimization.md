# In-memory log buffer optimization audit

Work branch: `update`. Based on tail optimization commit
`2454c017b9d1d12c3107bed80887ea383de4b0ed`.

## Change and compatibility

`GetLogs(1, ...)` copied all 4096 buffered records before selecting one.
Once full, the writer discarded a slice prefix and appended, periodically
allocating and copying a new backing array. Both paths now use a circular
buffer: writes replace the oldest entry and reads snapshot only the requested
matching entries. Formatting remains outside the mutex. Timestamp formatting
and level parsing for writes also occur before acquiring the buffer mutex.

The repeated anonymous record declarations were replaced by one named type.
The read path no longer needs `fmt.Sprintf` or its boxed formatting arguments
for the fixed output format. These changes preserve newest-first order,
severity thresholds, the 4096-entry limit, empty-query behavior, and the
existing timestamp/severity/message format.

## Verification

- Existing logger tests and three new behavioral tests pass. New cases cover
  more than three buffer wraps, six count/severity combinations, concurrent
  writers/readers, an empty buffer and an error-only query containing no matches.
- `go test -race -shuffle=on ./internal/logger ./internal/util/tail -count=1`
  passes after the final implementation change.
- `go vet ./internal/logger ./internal/util/tail` passes.
- `go test -json ./internal/web/service -run '^Test(Diagnostic|GetLogsSyslog|XrayAccessLog)' -count=1`
  completes with 34 passing test/subtest events, no skips or failures, and
  a passing package result. This includes the dashboard merge of persistent
  and in-memory records and legacy syslog consumers.
- Files formatted with gofumpt v0.12.0; `git diff --check` passes.

## Measurements

Same Linux amd64 environment, Go 1.27.3, before and after:
`go test ./internal/logger -count=1 -bench BenchmarkLogBuffer -benchmem -benchtime=200ms`.

- Write to full buffer: 224.0 to 188.5 ns/op; 197 to 24 B/op.
- Read one entry: 56,092 to 113.0 ns/op; 164,016 to 112 B/op;
  5 to 3 allocations/op.
- Read 100 entries: 74,557 to 5,833 ns/op; 173,711 to 10,688 B/op;
  302 to 102 allocations/op.

These are individual local microbenchmark samples, not claims about total
panel throughput. The writer still allocates its formatted timestamp; the
optimization removes buffer maintenance allocations and excessive snapshots.

## Audit progress

Two focused optimization changes are prepared on `update`: bounded file-tail
reading and buffered panel-log handling. Four production functions have
received focused change/contract/test review: `ReadTailLines`,
`RecentOutputBuffer.Write`, `addToBuffer`, and `GetLogs`.

The larger stage-1 checklist remains in progress. Source/path inventory and
initial test candidates do not establish semantic coverage for the remaining
files. Remaining work includes merge/deletion review, exact change-to-assertion
mapping, image/release binary digests and broader baseline fixtures. Full CI
is separate from the successful local focused checks reported here.
