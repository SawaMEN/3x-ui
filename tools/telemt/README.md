# Official Telemt in 3x-ui

The project now uses unmodified binaries from the official
[telemt/telemt releases](https://github.com/telemt/telemt/releases).
The previous Rust patch series, custom build suffix and patched-engine
release artifacts are retired. The standalone service, panel MTProto
inbounds and WEB Proxy remain managed by 3x-ui.

For each panel release, `tools/telemt/fetch.py` resolves a single stable
official tag across amd64 and arm64, downloads the upstream Linux musl
binary and checks its published SHA-256 sidecar, GitHub release digest
when available, archive layout, ELF architecture and native executable
version. The release packages that verified unmodified binary.

The installed `telemt-update.sh` downloads directly from
`telemt/telemt/releases`, validates the upstream checksum and executable
version before stopping services, and attempts rollback on installation
or restart failure. Legacy `3.5.14+3xui.1` installs migrate to official
`3.5.14` or newer stable upstream.

This drops patched-only Middle Proxy retry, writer-pool and fallback
changes. Panel-side MEKO, configuration, WEB carrier selection, Caddy,
HAProxy and nginx integrations are retained wherever official Telemt
supports them.

Run `python3 -m unittest discover -s tools -p test_official_telemt.py`
for offline source/checksum and updater regressions. Native API and WEB
integration tests run against upstream binaries on both architectures.

Cover-site acceptance checks use an official Telemt process and Chromium:

```sh
cd frontend && npm ci && npx playwright install --with-deps chromium && cd ..
python3 tools/telemt/fetch.py --arch amd64 --output /tmp/telemt-cover
TELEMT_TEST_BINARY=/tmp/telemt-cover/telemt TELEMT_SITE_BROWSER_SCRIPT="$PWD/tools/telemt/test_site.mjs" go test ./internal/web/service -run '^TestNativeTelemtWeb(CarrierConfig|SiteWithBasePath)$' -count=1 -v
```

They check page/resources, CSP errors, imported CSS/modules, and game controls.
A transport `base_path` does not move the public cover website out of `/`.
Imported owner HTML is converted to same-origin CSS/JS only for Telemt's strict
CSP; ordinary gateway sites retain the original document. Existing managed WEB
HTML snapshots are refreshed once during reconciliation, preserving old files.
