#!/usr/bin/env python3
"""Exercise release transaction wiring in temporary installations only."""
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
PANEL = r'''#!/bin/bash
set -eu
[[ "$1" == core-migrate ]]
shift
mode=apply
dir=""
while (( $# )); do
    case "$1" in
        --backup-dir) dir="$2"; shift 2;;
        --check-binary) mode=check; shift;;
        --backup-only) mode=backup; shift;;
        --restore) mode=restore; shift;;
        --verify-running) mode=verify; shift;;
        *) exit 3;;
    esac
done
printf '%s\n' "$mode" >> "$TEST_LOG"
case "$mode" in
check) [[ "${FAIL_PREFLIGHT:-0}" == 0 ]];;
backup) mkdir "$dir"; cp "$TEST_DB" "$dir/database";;
restore) cp "$dir/database" "$TEST_DB";;
apply) printf 'new-core\n' > "$TEST_DB"; [[ "${FAIL_APPLY:-0}" == 0 ]];;
verify) [[ "${FAIL_VERIFY:-0}" == 0 ]];;
esac
'''

class ReleaseMigrationTests(unittest.TestCase):
    def run_case(self, mode):
        with tempfile.TemporaryDirectory() as raw:
            base = Path(raw)
            old = base / "x-ui"
            (old / "bin").mkdir(parents=True)
            (old / "x-ui").write_text("old-panel")
            (old / "bin" / "custom-key").write_text("existing-private-key")
            (old / "bin" / "xray-linux-amd64").write_text("legacy-xray")
            (old / "bin" / "sing-box-linux-amd64").write_text("legacy-stock")
            (old / "bin" / "xray-linux-amd64").chmod(0o700)
            (old / "bin" / "sing-box-linux-amd64").chmod(0o700)
            if mode.startswith("external_"):
                external = base / "external-bin"
                external.mkdir()
                (external / "xray-linux-amd64").write_text("legacy-xray")
                (external / "sing-box-linux-amd64").write_text("legacy-stock")
                (external / "custom-key").write_text("external-key")
                (external / "xray-linux-amd64").chmod(0o700)
                (external / "sing-box-linux-amd64").chmod(0o700)
            database = base / "database"
            database.write_text("original-users-keys-traffic\n")
            unit = base / "unit"
            unit.write_text("old-unit")
            bundle = base / "bundle" / "x-ui"
            (bundle / "bin").mkdir(parents=True)
            (bundle / "x-ui").write_text(PANEL)
            (bundle / "x-ui").chmod(0o700)
            (bundle / "bin" / "hiddify-core-linux-amd64").write_text("owner-core")
            archive = base / "release.tar.gz"
            with tarfile.open(archive, "w:gz") as target:
                target.add(bundle, arcname="x-ui")
            script = r'''
set -eu
xui_folder="$TEST_ROOT/x-ui"
xui_service="$TEST_ROOT"
release=debian
if [[ "$CASE" == external_* ]]; then
    export XUI_BIN_FOLDER="$TEST_ROOT/external-bin"
    CASE="${CASE#external_}"
fi
arch() { printf amd64; }
systemctl() {
    printf 'service:%s\n' "$*" >> "$TEST_LOG"
    case "$1" in
        is-active) [[ "${SIM_RUNNING:-1}" == 1 ]];;
        is-enabled) [[ "${SIM_ENABLED:-1}" == 1 ]];;
        stop) [[ "$CASE" != stop_failure ]];;
        *) return 0;;
    esac
}
source "$HELPER"
core_migration_host_paths() { cm_host_paths=("$TEST_ROOT/unit"); }
core_migration_resume_gateway() { printf "resume-gateway\n" >> "$TEST_LOG"; }
if [[ "$CASE" == preflight ]]; then
    export FAIL_PREFLIGHT=1
    if core_migration_prepare_archive "$ARCHIVE"; then exit 21; fi
    core_migration_exit 2
    [[ "$(cat "$TEST_DB")" == original-users-keys-traffic ]]
    [[ "$(cat "$xui_folder/x-ui")" == old-panel ]]
    exit 0
fi
core_migration_prepare_archive "$ARCHIVE"
core_migration_begin
# Simulate the release replacement and host-unit installation.
rm -rf "$xui_folder"
cp -a "$TEST_ROOT/bundle/x-ui" "$xui_folder"
printf 'new-unit' > "$TEST_ROOT/unit"
if [[ -n "${XUI_BIN_FOLDER:-}" && "$XUI_BIN_FOLDER" != "$xui_folder/bin" ]]; then
    cp "$xui_folder/bin/hiddify-core-linux-amd64" "$XUI_BIN_FOLDER/"
fi
# Custom-file restoration can bring old engines back; commit must remove them.
printf 'legacy-xray' > "$xui_folder/bin/xray-linux-amd64"
printf 'legacy-stock' > "$xui_folder/bin/sing-box-linux-amd64"
chmod 0700 "$xui_folder/bin/xray-linux-amd64" "$xui_folder/bin/sing-box-linux-amd64"
printf 'custom-config' > "$xui_folder/bin/xray-config.json"
printf 'custom-config' > "$xui_folder/bin/sing-box-config.json"
if [[ "$CASE" == apply || "$CASE" == stopped_failure ]]; then export FAIL_APPLY=1; fi
if core_migration_apply; then
    if [[ "$CASE" == verify || "$CASE" == stop_failure ]]; then export FAIL_VERIFY=1; fi
    if core_migration_commit; then
        [[ "$CASE" == success || "$CASE" == disabled ]]
        [[ "$(cat "$TEST_DB")" == new-core ]]
        [[ -f "$cm_backup/installation/bin/custom-key" ]]
        [[ -f "$cm_backup/data/database" ]]
        [[ -f "$cm_backup/installation/bin/xray-linux-amd64" ]]
        [[ -f "$cm_backup/installation/bin/sing-box-linux-amd64" ]]
        [[ ! -e "$xui_folder/bin/xray-linux-amd64" ]]
        [[ ! -e "$xui_folder/bin/sing-box-linux-amd64" ]]
        [[ -f "$xui_folder/bin/hiddify-core-linux-amd64" ]]
        [[ "$(cat "$xui_folder/bin/xray-config.json")" == custom-config ]]
        [[ "$(cat "$xui_folder/bin/sing-box-config.json")" == custom-config ]]
        if [[ -d "$cm_backup/managed-bin" ]]; then
            [[ -f "$cm_backup/managed-bin/xray-linux-amd64" ]]
            [[ -f "$cm_backup/managed-bin/sing-box-linux-amd64" ]]
            [[ ! -e "$XUI_BIN_FOLDER/xray-linux-amd64" ]]
            [[ ! -e "$XUI_BIN_FOLDER/sing-box-linux-amd64" ]]
            [[ "$(cat "$XUI_BIN_FOLDER/custom-key")" == external-key ]]
        fi
        core_migration_exit 0
        exit 0
    fi
fi
[[ "$CASE" != success && "$CASE" != disabled ]]
if [[ "$CASE" == stop_failure ]]; then
    if core_migration_exit 2; then exit 22; fi
    [[ "$(cat "$TEST_DB")" == new-core ]]
    [[ -f "$cm_backup/installation/bin/custom-key" ]]
    [[ -d "$cm_stage" ]]
    exit 0
fi
core_migration_exit 2
[[ "$(cat "$TEST_DB")" == original-users-keys-traffic ]]
[[ "$(cat "$xui_folder/x-ui")" == old-panel ]]
[[ "$(cat "$xui_folder/bin/custom-key")" == existing-private-key ]]
[[ "$(cat "$TEST_ROOT/unit")" == old-unit ]]
[[ -d "$cm_backup/failed-installation" ]]
[[ -f "$cm_backup/data/database" ]]
[[ -f "$xui_folder/bin/xray-linux-amd64" ]]
[[ -f "$xui_folder/bin/sing-box-linux-amd64" ]]
if [[ -d "$cm_backup/managed-bin" ]]; then
    [[ "$(cat "$XUI_BIN_FOLDER/xray-linux-amd64")" == legacy-xray ]]
    [[ "$(cat "$XUI_BIN_FOLDER/sing-box-linux-amd64")" == legacy-stock ]]
    [[ "$(cat "$XUI_BIN_FOLDER/custom-key")" == external-key ]]
fi
'''
            env = dict(os.environ, TEST_ROOT=raw, TEST_DB=str(database),
                       TEST_LOG=str(base / "commands"), HELPER=str(ROOT / "core-migration.sh"),
                       ARCHIVE=str(archive), CASE=mode, SIM_ENABLED="0" if mode=="disabled" else "1",
                       SIM_RUNNING="0" if mode=="stopped_failure" else "1")
            result = subprocess.run(["bash", "-c", script], env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            commands = (base / "commands").read_text()
            mode = mode.removeprefix("external_")
            if mode != "preflight":
                self.assertIn("backup\n", commands)
                if mode == "stop_failure":
                    self.assertNotIn("restore\n", commands)
                    self.assertNotIn("service:start x-ui", commands)
                elif mode not in ("success", "disabled"):
                    self.assertIn("restore\n", commands)
                    if mode=="stopped_failure": self.assertNotIn("service:start x-ui",commands)
                    else: self.assertIn("service:start x-ui", commands)
                if mode=="disabled": self.assertIn("service:disable x-ui", commands)

    def test_success_retains_recovery_snapshot(self): self.run_case("success")
    def test_external_bin_cleanup_retains_snapshot(self): self.run_case("external_success")
    def test_external_bin_rollback_restores_legacy_files(self): self.run_case("external_verify")
    def test_apply_failure_recovers_installation_and_data(self): self.run_case("apply")
    def test_daemon_core_failure_recovers_installation_and_data(self): self.run_case("verify")
    def test_preflight_failure_does_not_touch_installation(self): self.run_case("preflight")
    def test_disabled_autostart_survives_upgrade(self): self.run_case("disabled")
    def test_rollback_keeps_originally_stopped_panel_stopped(self): self.run_case("stopped_failure")
    def test_failed_service_stop_blocks_restore(self): self.run_case("stop_failure")
    def test_both_installers_arm_rollback_before_replacement(self):
        for name in ("install.sh", "update.sh"):
            source = (ROOT / name).read_text()
            self.assertIn("core_migration_prepare_archive", source)
            self.assertIn("core_migration_begin", source)
            self.assertIn("core_migration_apply", source)
            self.assertIn("core_migration_commit", source)
            self.assertIn("core_migration_exit", source)
        self.assertNotIn("trap - EXIT INT TERM", (ROOT / "install.sh").read_text())

if __name__ == "__main__": unittest.main()
