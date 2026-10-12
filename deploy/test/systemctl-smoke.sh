#!/usr/bin/env bash
# The smoke container has no systemd PID 1. Supervise the real panel process
# here; unit syntax and host systemd integration are checked separately.
set -euo pipefail
command_name=${1:-}
shift || true
unit=""
for argument in "$@"; do
    [[ "$argument" == -* ]] || unit="$argument"
done
unit=${unit%.service}
[[ "$unit" == x-ui ]] || exit 0
pid_file=/tmp/xui-smoke.pid
enabled_file=/tmp/xui-smoke.enabled
active() {
    [[ -f "$pid_file" ]] && kill -0 "$(cat "$pid_file")" 2>/dev/null
}
stop() {
    if active; then
        local pid
        pid=$(cat "$pid_file")
        kill "$pid" 2>/dev/null || true
        for ((attempt=0; attempt<100; attempt++)); do
            kill -0 "$pid" 2>/dev/null || break
            sleep 0.1
        done
        kill -9 "$pid" 2>/dev/null || true
    fi
    rm -f "$pid_file"
}
start() {
    active && return 0
    (
        cd /usr/local/x-ui
        exec ./x-ui
    ) </dev/null >/tmp/xui.log 2>&1 &
    echo "$!" > "$pid_file"
    sleep 1
    active
}
case "$command_name" in
    start) start ;;
    stop) stop ;;
    restart) stop; start ;;
    is-active|status) active ;;
    is-enabled) [[ -f "$enabled_file" ]] ;;
    enable) touch "$enabled_file" ;;
    disable) rm -f "$enabled_file" ;;
    *) exit 0 ;;
esac
