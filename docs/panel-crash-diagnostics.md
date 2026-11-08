# Panel crash and availability diagnostics

The panel and subscription HTTP routers record recovered request panics and stack
frames in `3xui.log` and the application-log viewer. Requests after a panic keep
working. Diagnostics identify the HTTP method and route pattern; request headers,
bodies, query strings and subscription tokens are not dumped.

Standard Go logger output and `net/http` internal errors also go through the panel
logger. The configured log folder is `XUI_LOG_FOLDER` (normally `/var/log/x-ui`).
Existing log size/age rotation applies to these messages.

Fatal Go runtime reports, including an unrecovered background-goroutine panic,
are duplicated to `panic.log` in that folder. After the next successful process
lease acquisition, the last 64 KiB of the report is copied into `3xui.log` as ERROR
entries, then `panic.log` is reset for the new process. This does not suppress
fatal crashes or resume a potentially corrupted process; the service manager's
existing restart policy handles them.

An unexpected panel listener failure now requests an in-process panel restart
through the existing restart hook. Normal shutdown does not trigger recovery.
The tunnel monitor captures its core restart method before panel reloads, avoiding
a concurrent read of the replaceable server variable.

## If the panel becomes unavailable again

Record the incident time and inspect both application and OS logs:

```sh
journalctl -u x-ui --since '30 minutes ago' --no-pager
journalctl -k --since '30 minutes ago' --no-pager | grep -Ei 'oom|out of memory|killed process'
systemctl show x-ui -p ActiveState -p SubState -p Result -p NRestarts -p ExecMainStatus
tail -n 200 /var/log/x-ui/3xui.log
tail -n 200 /var/log/x-ui/panic.log
```

Use the configured log directory if different. SIGKILL, kernel OOM kills, disk
failure and power loss cannot produce a Go crash report; systemd/kernel logs are
needed for those incidents. These code fixes address lost diagnostics and a dead
HTTP listener; they do not establish the cause of a specific host's outage.
