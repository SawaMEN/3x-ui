# Panel failure diagnostics

The production panel records failed operations without enabling the debug
access log. Open the panel log view, or read `3xui.log` in `XUI_LOG_FOLDER`
(default: `/var/log/x-ui` on Linux). File rotation and the bounded UI buffer
remain enabled; this is recent diagnostic history, not unlimited archival.

Captured failure boundaries:

- HTTP 4xx/5xx responses, including authentication and permission denials,
  Gin errors and request cancellations. API route templates identify operations;
  request parameters, query strings and the private panel base path are excluded.
- Application failures returned through the JSON helpers, including failures
  carried in an HTTP 200 response. These are logged at ERROR, with the handler
  and source location. Already logged failures are not logged a second time by
  the outer HTTP observer.
- Outbound node RPC failures: node ID, code operation, HTTP method, elapsed
  milliseconds and a safe cause. Remote HTTP status and API rejection are
  preserved without dumping remote response bodies or token-bearing URLs.
  Malformed optional traffic snapshots are no longer ignored silently.
- Certificate-library warnings and errors from CertMagic, previously written
  only to stderr, now also reach the panel file and UI log buffer.
- Certificate source discovery, delivery/apply retries, renewal scans,
  persistence and certificate health/audit failures, plus failed SNI scans
  and SNI history writes.
- Log-file write failures preserve the original record in stderr (the service
  journal) and report the storage failure without recursively using the logger.
- Request panics retain stack traces. Existing runtime crash capture keeps
  fatal process/goroutine reports in `panic.log` and replays them on restart.

Warnings such as impending certificate expiry remain WARNING. Operational
failures are ERROR and are visible with the error-only log filter. Successful
HTTP requests are not logged by this observer.

Diagnostics exclude request/response objects and redact common credential,
PEM and private-URI formats. Individual diagnostic strings are bounded to
avoid injecting multiline records or unbounded messages into the log. This
is not a guarantee that every third-party component emits a diagnostic, nor
a replacement for a server-side log collector if long-term retention is needed.

## Dashboard and native-core crashes

The top dashboard log window includes `core/<engine>/console` under
**Core / crashes / stderr** and **All sources**. Child stdout/stderr is saved
in `<core-config-path>.console.log`, separately from the structured core log.
This retains Go panic stacks and diagnostics from libraries which bypass the
core logger. It remains visible after switching cores. Files rotate at 10 MiB
with two backups and seven-day retention; the viewer reads the active segment.

Multiline frames are grouped before search/severity filters and display limits.
Native timezone prefixes, coloured levels, and JSON severity/timestamps are
recognized. Generated native logging is enabled, uses timestamps by default,
and disables ANSI colours. Existing severity thresholds remain in effect.

Unexpected child exits generate a panel ERROR with PID, exit reason and console
path. Crash-loop backoff is retained across successful starts until the core
stays healthy for 30 seconds. Repeated traffic-triggered crashes therefore keep
increasing retry delays up to two minutes, rather than repeatedly treating
500 ms of successful startup as recovery.

After updating from a version without console capture, restart the core once
from the panel: an already-running external process cannot have stdout/stderr
reattached retroactively. SIGKILL/OOM cannot produce a Go stack; the exit signal
is recorded, while identifying an OOM kill requires kernel/systemd records.
No particular protocol is assumed to be responsible without crash evidence.
