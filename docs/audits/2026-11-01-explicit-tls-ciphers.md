# Explicit TLS cipher compatibility

The release build for `c893fe1f5765f67021cc0547c59335f8a3e9be90`
failed in AMD64 constructor checks, despite passing the normal CI matrix.
The panel preserves explicitly configured legacy Go TLS cipher suites,
but the pinned core's STD server/client and uTLS client only searched
`tls.CipherSuites()` and rejected RSA suites returned by
`tls.InsecureCipherSuites()`.

The new build overlay resolves explicitly supplied names through a shared
read-only registry of both Go lists. Empty/nil input keeps the library's
unchanged defaults; unknown names still fail, and order/duplicates are
preserved. This also removes three duplicated nested scans and repeated
cipher-list allocations. No cipher is enabled without explicit selection.

Regression checks cover defaults, empty input, modern/legacy/mixed lists,
duplicates, unknown names, every Go TLS suite, and actual STD/uTLS client
construction. The pinned build runs these tests alongside its existing
native transport/TLS tests. Its advertised version includes `ciphers1`
to distinguish it from the incompatible earlier binary.

Local pinned build completed and its generated-config constructor suite
passed all 10 test/subtest events without skips, including the three cases
that failed in the release job. The original failing tests were preserved.

Reference failure: release run 41294343665, job 124814038980,
`TestPinnedBinaryChecksGeneratedConfigurations`. A new release build on
the published fix must still be checked before signing off the release
work package; local success is not a claim that all architectures passed.

The separately downloaded latest kernel also rejected `final_mask` and
legacy suites in this probe. Its release identity is distinct from the
panel's pinned build with overlays; the two binaries must not be treated
as interchangeable in later compatibility and update checks.
