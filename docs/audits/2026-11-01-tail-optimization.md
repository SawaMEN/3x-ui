# Log tail optimization audit

Baseline: `4aa744111b4649be719e18a48166a20debd85038` on `main`.

## Change and reason

`ReadTailLines` repeatedly joined the entire partial line with each older
64 KiB chunk. A long or unterminated record therefore caused quadratic
copying even though file reads were bounded. Reading a 2 MiB tail allocated
about 46.6 MB in the benchmark below.

The reader now retains chunk fragments and assembles each complete record
once. Ordinary records no longer require an additional combined chunk
buffer. Byte and line budgets, newest-first ordering, CRLF handling, short
read errors, and rejection of a truncated oldest record remain unchanged.
An unreachable nonempty-write check was also removed from the zero-capacity
command-output buffer branch; empty writes return earlier.

## Verification

- Existing tail tests and 40 new byte-budget/line-limit combinations pass.
  The new cases compare chunked reads against a whole-string reference and
  include multi-chunk records, CRLF, blank lines and an unterminated last line.
- `go test -race ./internal/util/tail ./internal/logger -count=1` passes.
- `go vet ./internal/util/tail ./internal/logger` passes.
- Diagnostic and legacy log API regression selection passes: 34 test and
  subtest pass events, no skips or failures, using
  `go test -json ./internal/web/service -run '^Test(Diagnostic|GetLogsSyslog|XrayAccessLog)' -count=1`.
- Modified Go files were formatted with gofumpt v0.12.0; `git diff --check`
  passes. A full repository CI/build matrix has not been run in this pass.

## Local measurements

Measured with Go 1.27.3 on Linux amd64, using
`go test ./internal/util/tail -count=1 -bench BenchmarkReadTailLines -benchmem -benchtime=200ms`.
The same benchmark and tests were run before and after the implementation change.

- Short records: 149,448 to 115,258 ns/op; 512,165 to 323,728 B/op.
- Oversized record followed by a newest short line, 2 MiB byte budget:
  42,795,661 to 928,653 ns/op; 46,617,784 to 2,104,060 B/op.
- Oversized unterminated record, 2 MiB byte budget:
  41,830,058 to 966,784 ns/op; 46,617,813 to 2,104,049 B/op.

These are single local benchmark runs, not end-to-end panel performance
claims. The oversized records exceed the byte budget, so their incomplete
oldest content is correctly discarded in both versions.

## Overall audit status

This completes one measured optimization and its focused regression checks.
It does not close an entire work package of the 14-stage, 70-group audit.
Stage 1 still requires semantic change-to-assertion mapping, merge/deletion
review, release image/binary digests and broader migration fixtures.
Removal candidates must be checked for data, configuration and API compatibility;
existing migrations and protocol readers must not be removed solely because
their callers are uncommon.
