#!/bin/bash
# Sourced by both release installers. Every backup is retained for recovery.
# The caller owns its EXIT status/reporting trap and calls core_migration_exit.
core_migration_prepare_archive() {
    local archive="$1"
    exec {cm_lock}>"${xui_folder%/*}/.x-ui-upgrade.lock" || return 1
    flock -n "$cm_lock" || return 1
    cm_stage=$(mktemp -d "${xui_folder%/*}/.x-ui-release.XXXXXX") || return 1
    chmod 0700 "$cm_stage" || return 1
    tar -xzf "$archive" -C "$cm_stage" || return 1
    cm_was_running=0
    cm_was_enabled=0
    if [[ "$release" == alpine ]]; then
        rc-service x-ui status >/dev/null 2>&1 && cm_was_running=1
        rc-update show default 2>/dev/null | grep -q 'x-ui' && cm_was_enabled=1
    else
        systemctl is-active --quiet x-ui && cm_was_running=1
        systemctl is-enabled --quiet x-ui && cm_was_enabled=1
    fi
    [[ -x "$cm_stage/x-ui/x-ui" && -s "$cm_stage/x-ui/bin/hiddify-core-linux-$(arch)" ]] || return 1
    XUI_BIN_FOLDER="$cm_stage/x-ui/bin" "$cm_stage/x-ui/x-ui" core-migrate --check-binary || return 1
}
core_migration_begin() {
    [[ -n "${cm_stage:-}" && -d "$xui_folder" ]] || return 1
    export XUI_MAIN_FOLDER="$xui_folder"
    export XUI_BIN_FOLDER="${XUI_BIN_FOLDER:-$xui_folder/bin}"
    [[ "$XUI_BIN_FOLDER" == /* ]] || export XUI_BIN_FOLDER="$xui_folder/$XUI_BIN_FOLDER"
    cm_stopped=1
    cm_backup=$(mktemp -d "${xui_folder%/*}/x-ui-upgrade.XXXXXX") || return 1
    chmod 0700 "$cm_backup" || return 1
    # Do not let two installers snapshot/remove the same stopped installation.
    cp -a "$xui_folder" "$cm_backup/installation" || return 1
    # Preserve configured binaries outside the installation snapshot too.
    if [[ "$XUI_BIN_FOLDER" != "$xui_folder/bin" || -L "$XUI_BIN_FOLDER" ]]; then
        cp -aH "$XUI_BIN_FOLDER" "$cm_backup/managed-bin" || return 1
    fi
    mkdir "$cm_backup/host-files" || return 1
    core_migration_host_paths
    local i
    for i in "${!cm_host_paths[@]}"; do
        if [[ -e "${cm_host_paths[$i]}" ]]; then
            cp -a "${cm_host_paths[$i]}" "$cm_backup/host-files/$i" || return 1
        fi
    done
    "$cm_stage/x-ui/x-ui" core-migrate --backup-only --backup-dir "$cm_backup/data" || return 1
    cm_active=1
    echo "Offline installation snapshot: $cm_backup"
}
core_migration_apply() {
    [[ "${cm_active:-0}" == 1 ]] || return 0
    "$xui_folder/x-ui" core-migrate --backup-dir "$cm_backup/data"
}
core_migration_commit() {
    "$xui_folder/x-ui" core-migrate --verify-running || return 1
    core_migration_resume_gateway || return 1
    if [[ "${cm_was_enabled:-0}" == 0 ]]; then
        if [[ "$release" == alpine ]]; then rc-update del x-ui || return 1; else systemctl disable x-ui || return 1; fi
    fi
    # Legacy executables are removed only after the owner runtime is verified.
    # The installation snapshot keeps them available for recovery.
    local legacy root
    for root in "$xui_folder/bin" "$XUI_BIN_FOLDER"; do
        for legacy in "$root"/xray "$root"/xray-* "$root"/sing-box "$root"/sing-box-*; do
            [[ ! -f "$legacy" || ! -x "$legacy" ]] || rm -f -- "$legacy" || return 1
        done
    done
    cm_active=0
    cm_stopped=0
    [[ -z "${cm_stage:-}" ]] || rm -rf -- "$cm_stage"
    echo "Upgrade committed; recovery snapshot retained: ${cm_backup:-none}"
}
core_migration_exit() {
    local code="$1" incomplete=0
    [[ "${cm_active:-0}" != 1 || "$code" != 0 ]] || incomplete=1
    if [[ "${cm_active:-0}" == 1 ]]; then
        echo "Upgrade failed; restoring offline snapshot: $cm_backup" >&2
        if [[ "${release}" == alpine ]]; then
            rc-service x-ui stop || return 1
        else
            systemctl stop x-ui || return 1
        fi
        if ! "$cm_stage/x-ui/x-ui" core-migrate --restore --backup-dir "$cm_backup/data"; then
            echo "Automatic rollback failed. Keep x-ui stopped and recover from $cm_backup." >&2
            return 1
        fi
        # Do not lose the failed installation; it can be inspected separately.
        if [[ -d "$xui_folder" ]]; then mv "$xui_folder" "$cm_backup/failed-installation" || return 1; fi
        cp -a "$cm_backup/installation" "$xui_folder" || return 1
        if [[ -d "$cm_backup/managed-bin" ]]; then
            mkdir -p "$XUI_BIN_FOLDER" || return 1
            cp -a "$cm_backup/managed-bin/." "$XUI_BIN_FOLDER/" || return 1
        fi
        local i
        for i in "${!cm_host_paths[@]}"; do
            if [[ -e "$cm_backup/host-files/$i" ]]; then
                cp -a "$cm_backup/host-files/$i" "${cm_host_paths[$i]}" || return 1
            else
                rm -f -- "${cm_host_paths[$i]}" || return 1
            fi
        done
        if [[ "${release}" == alpine ]]; then
            if [[ "${cm_was_enabled:-0}" == 1 ]]; then rc-update add x-ui || return 1; else rc-update del x-ui || return 1; fi
            [[ "${cm_was_running:-0}" == 0 ]] || rc-service x-ui start || return 1
        else
            systemctl daemon-reload || return 1
            if [[ "${cm_was_enabled:-0}" == 1 ]]; then systemctl enable x-ui || return 1; else systemctl disable x-ui || return 1; fi
            [[ "${cm_was_running:-0}" == 0 ]] || systemctl start x-ui || return 1
        fi
        if [[ "${cm_was_running:-0}" == 1 ]]; then core_migration_resume_gateway || return 1; fi
        cm_active=0
        cm_stopped=0
    elif [[ "${cm_stopped:-0}" == 1 ]]; then
        if [[ "${cm_was_running:-0}" == 1 ]]; then
            if [[ "$release" == alpine ]]; then rc-service x-ui start || return 1; else systemctl start x-ui || return 1; fi
        fi
        cm_stopped=0
    fi
    [[ -z "${cm_stage:-}" ]] || rm -rf -- "$cm_stage"
    [[ "$incomplete" == 0 ]]
}

core_migration_host_paths() {
    cm_host_paths=("/usr/bin/x-ui" "/etc/init.d/x-ui" "${xui_service}/x-ui.service"
        "${xui_service}/telemt.service" "${xui_service}/telemt-meko-fix.service"
        "${xui_service}/telemt-update.service" "${xui_service}/telemt-update.timer")
}

core_migration_resume_gateway() {
    # Restore interception only after the panel owns its listeners; the persisted
    # script checks ownership comments, routing and both TCP/UDP readiness.
    if [[ -f /etc/x-ui/gateway.env ]] && grep -qx 'NETWORK_ENABLED=1' /etc/x-ui/gateway.env; then
        [[ -s /usr/local/sbin/xui-gateway-firewall-restore ]] || return 1
        bash /usr/local/sbin/xui-gateway-firewall-restore routing || return 1
        bash /usr/local/sbin/xui-gateway-firewall-restore firewall || return 1
    fi
}
