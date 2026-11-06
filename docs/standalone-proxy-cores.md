# Standalone proxy cores

Open **Settings → Additional settings → Proxy Core**. The bundled owner kernel
remains the default. On the sing-box or Xray card, choose **Download and install**,
then **Use**. Selection takes effect immediately and persists across panel restarts.
Existing connections restart when the engine changes.

Automatic installation supports Linux. The installer resolves a stable release
from `SagerNet/sing-box` or `XTLS/Xray-core`, chooses the architecture-specific
archive, verifies its published SHA-256, checks the staged executable identity,
and atomically replaces the separate binary. It never replaces the bundled core.
Missing Xray `geoip.dat` and `geosite.dat` files are installed from that same
verified archive; existing databases are preserved. To update an active standalone
engine, first switch to another engine, then choose **Update**.

Before switching, the panel generates and validates a complete configuration with
the requested engine. Unsupported configuration or failed startup restores the
previous selection. Enabled Gateway state is migrated by the existing switch
transaction. When rollback follows a runtime failure, the previous engine is
restarted only if it was running before the switch.

## Official sing-box limitations

The official sing-box release omits `with_v2ray_api`. Its configuration therefore
uses the Clash API instead of the owner kernel's statistics/session RPCs. Exact
native per-user traffic counters and traffic-volume quotas are unavailable in this
mode. Live connection snapshots cannot replace cumulative counters because they
omit closed connections. The card explains this limitation. Use the bundled
kernel or Xray when exact per-user traffic quotas are required. Protocol sidecars
continue using their own existing collectors.

Owner-specific protocols/options are not assumed to work in upstream binaries.
The chosen executable's configuration check is authoritative. Unsupported settings
must be removed or the bundled engine selected before switching.

## API

All routes use the existing authenticated settings API group.

- `GET /panel/api/setting/cores`: installed/running/version/selection for all engines.
- `POST /panel/api/setting/cores/:core/install`: optional JSON `{"version":"latest"}`;
  a specific upstream release tag can also be supplied.
- `POST /panel/api/setting/cores/:core/select`: validate, select, and start the engine.

Core identifiers: `sing-box` is the bundled owner kernel; `sing-box-upstream` is
separately installed official sing-box; `xray` is separately installed Xray.
The legacy `hiddify-core` database alias continues mapping to the bundled kernel.

`TestStandaloneReleaseConfig` optionally accepts `XUI_TEST_XRAY_BINARY` and
`XUI_TEST_SINGBOX_BINARY`. The Xray executable must have the release's geodata
files beside it. These tests validate generated configuration, start the official
executables, and stop them without contacting public IP services. Environments
which prohibit netlink sockets skip the upstream sing-box startup after its
configuration check succeeds.
