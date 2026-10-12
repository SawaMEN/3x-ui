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
apply) [[ -x "$XUI_BIN_FOLDER/hiddify-core-linux-amd64" ]]; printf 'new-core\n' > "$TEST_DB"; [[ "${FAIL_APPLY:-0}" == 0 ]];;
verify) [[ "${FAIL_VERIFY:-0}" == 0 ]];;
esac
'''

class ReleaseMigrationTests(unittest.TestCase):
    def run_case(self, mode, relative_link=False):
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
            if mode.startswith(("external_", "symlink_")):
                external = base / "external-bin"
                external.mkdir()
                (external / "xray-linux-amd64").write_text("legacy-xray")
                (external / "sing-box-linux-amd64").write_text("legacy-stock")
                (external / "custom-key").write_text("external-key")
                (external / "xray-linux-amd64").chmod(0o700)
                (external / "sing-box-linux-amd64").chmod(0o700)
            if mode.startswith("symlink_"):
                import shutil
                shutil.rmtree(old / "bin")
                (old / "bin").symlink_to("../external-bin" if relative_link else external, target_is_directory=True)
            database = base / "database"
            database.write_text("original-users-keys-traffic\n")
            unit = base / "unit"
            unit.write_text("old-unit")
            bundle = base / "bundle" / "x-ui"
            (bundle / "bin").mkdir(parents=True)
            (bundle / "x-ui").write_text(PANEL)
            (bundle / "x-ui").chmod(0o700)
            (bundle / "bin" / "hiddify-core-linux-amd64").write_text("modified-sing-box")
            (bundle / "bin" / "hiddify-core-linux-amd64").chmod(0o700)
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
if [[ "$CASE" == symlink_* ]]; then
    unset XUI_BIN_FOLDER
    CASE="${CASE#symlink_}"
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
        [[ ! -e "$cm_backup" ]] # The verified offline snapshot must not fill the VPS.
        [[ -f "$xui_folder/bin/custom-key" || -f "$XUI_BIN_FOLDER/custom-key" ]]
        [[ ! -e "$xui_folder/bin/xray-linux-amd64" ]]
        [[ ! -e "$xui_folder/bin/sing-box-linux-amd64" ]]
        [[ -f "$xui_folder/bin/hiddify-core-linux-amd64" ]]
        [[ "$(cat "$xui_folder/bin/xray-config.json")" == custom-config ]]
        [[ "$(cat "$xui_folder/bin/sing-box-config.json")" == custom-config ]]
        [[ ! -e "$XUI_BIN_FOLDER/xray-linux-amd64" ]]
        [[ ! -e "$XUI_BIN_FOLDER/sing-box-linux-amd64" ]]
        if [[ "$XUI_BIN_FOLDER" != "$xui_folder/bin" || -L "$xui_folder/bin" ]]; then
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
    [[ -f "$cm_backup/installation/bin/custom-key" || -f "$cm_backup/managed-bin/custom-key" ]]
    [[ -d "$cm_stage" ]]
    exit 0
fi
core_migration_exit 2
[[ "$(cat "$TEST_DB")" == original-users-keys-traffic ]]
[[ "$(cat "$xui_folder/x-ui")" == old-panel ]]
[[ "$(cat "$xui_folder/bin/custom-key")" == existing-private-key || "$(cat "$xui_folder/bin/custom-key")" == external-key ]]
[[ "$(cat "$TEST_ROOT/unit")" == old-unit ]]
[[ -d "$cm_backup/failed-installation" ]]
[[ -f "$cm_backup/data/database" ]]
[[ -f "$xui_folder/bin/xray-linux-amd64" ]]
[[ -f "$xui_folder/bin/sing-box-linux-amd64" ]]
if [[ -d "$cm_backup/managed-bin" ]]; then
    [[ ! -e "$XUI_BIN_FOLDER/hiddify-core-linux-amd64" ]]
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
            mode = mode.removeprefix("external_").removeprefix("symlink_")
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

    def test_relative_symlink_bin_success(self): self.run_case("symlink_success", relative_link=True)
    def test_relative_symlink_bin_rollback(self): self.run_case("symlink_verify", relative_link=True)
    def test_symlink_bin_success(self): self.run_case("symlink_success")
    def test_symlink_bin_rollback(self): self.run_case("symlink_verify")

    def test_success_removes_verified_recovery_snapshot(self): self.run_case("success")
    def test_external_bin_cleanup_removes_snapshot(self): self.run_case("external_success")
    def test_external_bin_rollback_restores_legacy_files(self): self.run_case("external_verify")
    def test_apply_failure_recovers_installation_and_data(self): self.run_case("apply")
    def test_daemon_core_failure_recovers_installation_and_data(self): self.run_case("verify")
    def test_preflight_failure_does_not_touch_installation(self): self.run_case("preflight")
    def test_disabled_autostart_survives_upgrade(self): self.run_case("disabled")
    def test_rollback_keeps_originally_stopped_panel_stopped(self): self.run_case("stopped_failure")
    def test_failed_service_stop_blocks_restore(self): self.run_case("stop_failure")
    def test_fresh_external_and_relative_bin_receive_release(self):
        for relative in (False, True):
            with self.subTest(relative=relative), tempfile.TemporaryDirectory() as raw:
                base = Path(raw)
                stage = base / "stage" / "x-ui" / "bin"
                stage.mkdir(parents=True)
                (stage / "hiddify-core-linux-amd64").write_text("modified-sing-box")
                (stage / "geoip.dat").write_text("release-geodata")
                install = base / "x-ui"
                install.mkdir()
                dest = install / "custom-bin" if relative else base / "external-bin"
                dest.mkdir()
                (dest / "custom.dat").write_text("custom-database")
                script = r'''set -eu
source "$HELPER"
arch() { printf amd64; }
xui_folder="$TEST_ROOT/x-ui"
cm_stage="$TEST_ROOT/stage"
core_migration_install_managed_bin
[[ -x "$XUI_BIN_FOLDER/hiddify-core-linux-amd64" ]]
[[ "$(cat "$XUI_BIN_FOLDER/geoip.dat")" == release-geodata ]]
[[ "$(cat "$XUI_BIN_FOLDER/custom.dat")" == custom-database ]]
'''
                env = dict(os.environ, HELPER=str(ROOT / "core-migration.sh"), TEST_ROOT=raw,
                           XUI_BIN_FOLDER="custom-bin" if relative else str(dest))
                result = subprocess.run(["bash", "-c", script], env=env, text=True, capture_output=True)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_both_installers_arm_rollback_before_replacement(self):
        for name in ("install.sh", "update.sh"):
            source = (ROOT / name).read_text()
            self.assertIn("core_migration_prepare_archive", source)
            self.assertIn("core_migration_begin", source)
            self.assertIn("core_migration_apply", source)
            self.assertIn("core_migration_commit", source)
            self.assertIn("core_migration_exit", source)
        self.assertNotIn("trap - EXIT INT TERM", (ROOT / "install.sh").read_text())

class ReleaseCompatibilityTests(unittest.TestCase):
    def test_release_selection_preserves_migration_guards(self):
        source = (ROOT / "install.sh").read_text()
        start = source.index("prepare_release_migration() {")
        function = source[start:source.index("\n}\n", start) + 3]
        for kind, installed, expected in (("legacy", False, 0), ("legacy", True, 1),
                                          ("broken-new", False, 1), ("new", False, 0)):
            with self.subTest(kind=kind, installed=installed), tempfile.TemporaryDirectory() as raw:
                base = Path(raw)
                bundle = base / "bundle" / "x-ui"
                (bundle / "bin").mkdir(parents=True)
                (bundle / "x-ui").write_text("panel")
                binary = "xray-linux-amd64" if kind == "legacy" else "hiddify-core-linux-amd64"
                (bundle / "bin" / binary).write_text("kernel")
                if kind == "new":
                    (bundle / "core-migration.sh").write_text(
                        'core_migration_prepare_archive() { touch "$TEST_PREFLIGHT"; }\n'
                        'core_migration_exit() { return 0; }\n')
                folder = base / "installed"
                (folder / "bin").mkdir(parents=True)
                if installed:
                    (folder / "bin" / "hiddify-core-linux-amd64").write_text("existing kernel")
                archive = base / "release.tar.gz"
                with tarfile.open(archive, "w:gz") as target:
                    target.add(bundle, arcname="x-ui")
                env = dict(os.environ, ARCHIVE=str(archive), xui_folder=str(folder),
                           TEST_PREFLIGHT=str(base / "preflight"))
                result = subprocess.run(["bash", "-c", function +
                                         '\narch() { echo amd64; }\nprepare_release_migration "$ARCHIVE"'],
                                        env=env, capture_output=True, text=True)
                self.assertEqual(result.returncode, expected, result.stderr)
                self.assertEqual((base / "preflight").exists(), kind == "new")
                if installed:
                    self.assertEqual((folder / "bin" / "hiddify-core-linux-amd64").read_text(), "existing kernel")

class UpgradeStorageTests(unittest.TestCase):
    def run_script(self, base, script):
        env = dict(os.environ, HELPER=str(ROOT / "core-migration.sh"), TEST_ROOT=str(base))
        result = subprocess.run(["bash", "-c", 'set -eu\nsource "$HELPER"\n' + script],
                                env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def test_verified_snapshots_are_removed_and_failed_backups_survive(self):
        with tempfile.TemporaryDirectory() as raw:
            base = Path(raw)
            marker = ".x-ui-upgrade-verified"
            for name in ("old", "current", "failed", "unmarked", "foreign"):
                folder = base / ("x-ui-upgrade." + name)
                folder.mkdir()
                (folder / "database").write_text("recovery-data")
            (base / "x-ui-upgrade.old" / marker).write_text("x-ui verified upgrade v1\n")
            (base / "x-ui-upgrade.foreign" / marker).write_text("unrecognized marker\n")
            (base / "x-ui-upgrade.link").symlink_to(base / "x-ui-upgrade.old", target_is_directory=True)
            self.run_script(base, r'''
xui_folder="$TEST_ROOT/x-ui"
cm_backup="$TEST_ROOT/x-ui-upgrade.current"
core_migration_prune_snapshots
[[ ! -e "$TEST_ROOT/x-ui-upgrade.old" ]]
[[ -L "$TEST_ROOT/x-ui-upgrade.link" ]]
[[ ! -e "$cm_backup" ]]
for name in failed unmarked foreign; do
    [[ -f "$TEST_ROOT/x-ui-upgrade.$name/database" ]]
done
# A subsequent success also removes its own snapshot.
mkdir "$TEST_ROOT/x-ui-upgrade.next"
cm_backup="$TEST_ROOT/x-ui-upgrade.next"
core_migration_prune_snapshots
[[ ! -e "$TEST_ROOT/x-ui-upgrade.current" ]]
[[ ! -e "$cm_backup" ]]
''')

    def test_low_space_rejects_release_before_staging_or_service_stop(self):
        with tempfile.TemporaryDirectory() as raw:
            base = Path(raw)
            install = base / "x-ui"
            install.mkdir()
            (install / "x-ui").write_text("old-panel")
            with tarfile.open(base / "release.tar.gz", "w:gz") as target:
                target.add(install, arcname="x-ui")
            result = self.run_script(base, r'''
xui_folder="$TEST_ROOT/x-ui"
XUI_DB_FOLDER="$TEST_ROOT/database"
df() { printf 'Filesystem 1024-blocks Used Available Capacity Mounted\nmock 100000 99999 1 99%% /\n'; }
systemctl() { touch "$TEST_ROOT/service-touched"; return 0; }
if core_migration_prepare_archive "$TEST_ROOT/release.tar.gz"; then exit 21; fi
[[ "$(cat "$xui_folder/x-ui")" == old-panel ]]
[[ ! -e "$TEST_ROOT/service-touched" ]]
[[ -z "${cm_stage:-}" ]]
core_migration_exit 1
''')
            self.assertIn("Insufficient disk space", result.stderr)

    def test_corrupt_archive_rejected_by_space_check(self):
        with tempfile.TemporaryDirectory() as raw:
            base = Path(raw)
            (base / "release.tar.gz").write_bytes(b"invalid-gzip")
            self.run_script(base, r'''
xui_folder="$TEST_ROOT/x-ui"
if core_migration_check_space "$TEST_ROOT/release.tar.gz"; then exit 21; fi
''')

if __name__ == "__main__": unittest.main()

