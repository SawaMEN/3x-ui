#!/bin/bash
# Sourced by both release installers. Keep one verified recovery snapshot;
# failed and unmarked snapshots are retained for manual recovery.
# The caller owns its EXIT status/reporting trap and calls core_migration_exit.
core_migration_check_space() {
    local archive="$1" parent="${xui_folder%/*}" expanded available required snapshot=0 size folder
    # Count the decompressed stream, including tar headers, without extracting
    # another copy. pipefail catches truncated archives before the panel stops.
    expanded=$(set -o pipefail; gzip -dc -- "$archive" | wc -c) || return 1
    [[ "$expanded" =~ ^[[:space:]]*[0-9]+[[:space:]]*$ ]] || return 1
    for folder in "$xui_folder" "${XUI_DB_FOLDER:-/etc/x-ui}"; do
        [[ -d "$folder" ]] || continue
        size=$(du -sk -- "$folder" | awk '{print $1}') || return 1
        [[ "$size" =~ ^[0-9]+$ ]] || return 1
        snapshot=$((snapshot + size))
    done
    folder="${XUI_BIN_FOLDER:-$xui_folder/bin}"
    [[ "$folder" == /* ]] || folder="$xui_folder/$folder"
    if [[ -d "$folder" && ( "$folder" != "$xui_folder/bin" || -L "$folder" ) ]]; then
        size=$(du -skL -- "$folder" | awk '{print $1}') || return 1
        [[ "$size" =~ ^[0-9]+$ ]] || return 1
        snapshot=$((snapshot + size))
    fi
    available=$(df -Pk -- "$parent" | awk 'END {print $4}') || return 1
    [[ "$available" =~ ^[0-9]+$ ]] || return 1
    # Staging, installed release, offline snapshot, and 64 MiB for filesystem
    # overhead/runtime writes. Existing recovery copies remain intact on failure.
    required=$((2 * (expanded / 1024 + 1) + snapshot + 65536))
    if (( available < required )); then
        echo "Insufficient disk space for a safe upgrade: need ${required} KiB, available ${available} KiB on $parent. Installation was not changed." >&2
        return 1
    fi
}

core_migration_prune_snapshots() {
    local old marker=".x-ui-upgrade-verified"
    [[ -n "${cm_backup:-}" && -d "$cm_backup" ]] || return 0
    # Only this helper's completed transactions are eligible. Never infer
    # success from directory names: failed rollbacks contain recovery data too.
    printf 'x-ui verified upgrade v1\n' > "$cm_backup/$marker" || return 1
    for old in "${xui_folder%/*}"/x-ui-upgrade.*; do
        [[ "$old" != "$cm_backup" && -d "$old" && ! -L "$old" ]] || continue
        [[ -f "$old/$marker" && ! -L "$old/$marker" ]] || continue
        [[ "$(cat "$old/$marker")" == 'x-ui verified upgrade v1' ]] || continue
        rm -rf -- "$old" || echo "Could not remove old verified snapshot: $old" >&2
    done
    return 0
}

core_migration_prepare_archive() {
    local archive="$1"
    exec {cm_lock}>"${xui_folder%/*}/.x-ui-upgrade.lock" || return 1
    flock -n "$cm_lock" || return 1
    core_migration_check_space "$archive" || return 1
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
    if [[ -L "$xui_folder/bin" ]]; then
        cm_original_bin_target=$(readlink -f "$xui_folder/bin") || return 1
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
    core_migration_install_managed_bin || return 1
    "$xui_folder/x-ui" core-migrate --backup-dir "$cm_backup/data"
}
core_migration_install_managed_bin() {
    # Releases extract into installation/bin; a configured external directory
    # must receive the same binaries and databases before the migration trial.
    export XUI_MAIN_FOLDER="$xui_folder"
    export XUI_BIN_FOLDER="${XUI_BIN_FOLDER:-$xui_folder/bin}"
    [[ "$XUI_BIN_FOLDER" == /* ]] || export XUI_BIN_FOLDER="$xui_folder/$XUI_BIN_FOLDER"
    if [[ -n "${cm_backup:-}" && -L "$cm_backup/installation/bin" && "$XUI_BIN_FOLDER" == "$xui_folder/bin" ]]; then
        # The installer extracts a real bin directory. Restore the configured
        # link before deploying the release into its original target.
        if [[ ! -L "$xui_folder/bin" ]]; then
            # Relative links in the snapshot resolve from a different parent.
            # Copy to the original resolved target before restoring the link.
            [[ -n "${cm_original_bin_target:-}" ]] || return 1
            mkdir -p "$cm_original_bin_target" || return 1
            cp -a "$xui_folder/bin/." "$cm_original_bin_target/" || return 1
            rm -rf -- "${xui_folder:?}/bin" || return 1
            cp -a "$cm_backup/installation/bin" "$xui_folder/bin" || return 1
        fi
    fi
    if [[ "$XUI_BIN_FOLDER" == "$xui_folder/bin" && ! -L "$XUI_BIN_FOLDER" ]]; then return 0; fi
    mkdir -p "$XUI_BIN_FOLDER" || return 1
    cp -a "$cm_stage/x-ui/bin/." "$XUI_BIN_FOLDER/" || return 1
    chmod +x "$XUI_BIN_FOLDER/hiddify-core-linux-$(arch)" || return 1
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
    # Cleanup is after verification and disarming rollback. A cleanup error
    # cannot turn a healthy release into an attempted rollback without staging.
    core_migration_prune_snapshots || echo "Could not mark recovery snapshot for retention cleanup: $cm_backup" >&2
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
            # Remove assets first introduced by this release as well as restore
            # overwritten files. Keep unrelated custom files intact.
            local asset rel
            while IFS= read -r -d '' asset; do
                rel="${asset#"$cm_stage/x-ui/bin/"}"
                if [[ ! -e "$cm_backup/managed-bin/$rel" && ! -L "$cm_backup/managed-bin/$rel" ]]; then
                    rm -f -- "$XUI_BIN_FOLDER/$rel" || return 1
                fi
            done < <(find "$cm_stage/x-ui/bin" \( -type f -o -type l \) -print0)
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

