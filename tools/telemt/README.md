# Telemt for 3x-ui

The panel distributes a ready-to-run, static musl engine for amd64 and arm64.
There is no Rust compilation on the user's VDS. The upstream revision and the
3x-ui build version are pinned in `source.json`; update both the patch and build
revision together when changing the engine.

`3x-ui.patch` adapts the MIT-licensed [MTProxyL engine series](https://github.com/Liafanx/MTProxyL/tree/d3c98f7d4f4971822e90bba5068a977140c1b106/engine/telemt/3.5.7)
to official Telemt 3.5.14 (`9d5b896bb695c55e2905b82f276da84e45a0a2ec`).
The original license is retained in `LICENSE-MTProxyL`; Telemt's own license
remains in the upstream checkout.

Included behavior:

- Failed ME refill attempts use capped exponential backoff with jitter.
- Normal Active-writer growth is capped per DC. Required coverage and bounded
  replacement admission retain Telemt 3.5.14's recovery semantics.
- Partial ME degradation sends sessions for an unavailable DC through Direct,
  while healthy DCs retain ME. Recovered Direct sessions return to ME through
  reconnects after the hysteresis window. Quota-incarnation handling, current
  endpoint snapshots and generation ownership from 3.5.14 are preserved.
- On Linux/glibc only, a periodic `malloc_trim(0)` runs on the blocking pool.
  musl builds do not call this function; no musl RAM reduction is claimed from
  that glibc-specific patch.

## Build

Requires Rust >=1.88, Python 3, Git and a native musl C toolchain. The release
workflow runs on each native architecture, tests before building, and checks
panel API reload and WEB carriers against the resulting binary.

```bash
rustup target add x86_64-unknown-linux-musl
bash tools/telemt/build.sh /tmp/3x-ui-telemt-source /tmp/3x-ui-telemt-output x86_64-unknown-linux-musl
```

Use a clean source directory at the pinned revision. A dirty checkout is
rejected rather than reset. The output contains `telemt`, `telemt.version`, an
architecture-specific tarball and its SHA-256 file. The panel package embeds
that same tested engine. No separate per-commit dev releases are created.

The rolling `dev-latest` release receives the standalone tarballs/checksums
first, then `telemt.version`. The updater validates checksums, manifest version
and executable version before stopping a service or replacing a binary. A
concurrent rolling upload fails safely and can be retried. It never silently
replaces the patched engine with an official unpatched build.

## Validation on 17 October 2026

- Patched Rust suite: 2528 passed, 72 upstream-ignored, 9 environment-dependent
  cases filtered locally. The sandbox blocks Unix sockets, changing ownership
  and interface binding needed by those cases. Release CI runs the unfiltered
  suite; a failed test prevents engine publication.
- Both regressions found while porting were corrected: stale same-family
  writers cannot block required coverage, and a live same-DC writer continues
  bridging preferred-endpoint refill.
- A native amd64 musl release build reports `3.5.14+3xui.1`.
- Native Go integration passed API stats, user addition and reload, plus an
  actual WEB listener with all four carriers configured.
- This is correctness and concurrency validation, not a benchmark of throughput
  or bypass effectiveness on a particular VDS/provider.
