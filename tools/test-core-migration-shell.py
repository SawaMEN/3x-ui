#!/usr/bin/env python3
"""Exercise release transaction wiring in temporary installations only."""
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import time
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
    def run_case(self, mode, relative_link=False, alpine=False):
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
release="${TEST_RELEASE:-debian}"
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
        cat) [[ "$CASE" != missing_service ]];;
        show) [[ "$CASE" != missing_unit_failure ]] || printf "not-found\n";;
        stop) [[ "$CASE" != stop_failure && "$CASE" != missing_unit_failure ]];;
        *) return 0;;
    esac
}
rc-service() {
    printf 'service:%s %s\n' "$2" "$1" >> "$TEST_LOG"
    case "$2" in
        status) [[ ! -e "$TEST_ROOT/replaced" && "${SIM_RUNNING:-1}" == 1 ]];;
        stop) [[ ! -e "$TEST_ROOT/replaced" ]];;
        *) return 0;;
    esac
}
rc-update() {
    case "$1" in
        show) printf 'x-ui | default\n';;
        *) printf 'autostart:%s\n' "$*" >> "$TEST_LOG";;
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
touch "$TEST_ROOT/replaced"
printf 'new-unit' > "$TEST_ROOT/unit"
# Custom-file restoration can bring old engines back; commit must remove them.
printf 'legacy-xray' > "$xui_folder/bin/xray-linux-amd64"
printf 'legacy-stock' > "$xui_folder/bin/sing-box-linux-amd64"
chmod 0700 "$xui_folder/bin/xray-linux-amd64" "$xui_folder/bin/sing-box-linux-amd64"
printf 'custom-config' > "$xui_folder/bin/xray-config.json"
printf 'custom-config' > "$xui_folder/bin/sing-box-config.json"
if [[ "$CASE" == apply || "$CASE" == stopped_failure || "$CASE" == missing_unit_failure ]]; then export FAIL_APPLY=1; fi
if core_migration_apply; then
    if [[ "$CASE" == verify || "$CASE" == stop_failure ]]; then export FAIL_VERIFY=1; fi
    if core_migration_commit; then
        [[ "$CASE" == success || "$CASE" == disabled || "$CASE" == missing_service ]]
        [[ "$(cat "$TEST_DB")" == new-core ]]
        [[ ! -e "$cm_backup" ]] # The verified offline snapshot must not fill the VPS.
        [[ -f "$xui_folder/bin/custom-key" || -f "$XUI_BIN_FOLDER/custom-key" ]]
        [[ -e "$xui_folder/bin/xray-linux-amd64" ]]
        [[ -e "$xui_folder/bin/sing-box-linux-amd64" ]]
        [[ -f "$xui_folder/bin/hiddify-core-linux-amd64" ]]
        [[ "$(cat "$xui_folder/bin/xray-config.json")" == custom-config ]]
        [[ "$(cat "$xui_folder/bin/sing-box-config.json")" == custom-config ]]
        [[ -e "$XUI_BIN_FOLDER/xray-linux-amd64" ]]
        [[ -e "$XUI_BIN_FOLDER/sing-box-linux-amd64" ]]
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
            env = dict(os.environ, TEST_ROOT=raw, TEST_DB=str(database), TEST_RELEASE="alpine" if alpine else "debian",
                       TEST_LOG=str(base / "commands"), HELPER=str(ROOT / "core-migration.sh"),
                       ARCHIVE=str(archive), CASE=mode, SIM_ENABLED="0" if mode in ("disabled", "missing_unit_failure", "missing_service") else "1",
                       SIM_RUNNING="0" if mode in ("stopped_failure", "missing_unit_failure", "missing_service") else "1")
            result = subprocess.run(["bash", "-c", script], env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            commands = (base / "commands").read_text()
            mode = mode.removeprefix("external_").removeprefix("symlink_")
            if mode != "preflight":
                self.assertIn("backup\n", commands)
                if mode == "stop_failure":
                    self.assertNotIn("restore\n", commands)
                    self.assertNotIn("service:start x-ui", commands)
                elif mode not in ("success", "disabled", "missing_service"):
                    self.assertIn("restore\n", commands)
                    if mode in ("stopped_failure", "missing_unit_failure"): self.assertNotIn("service:start x-ui",commands)
                    else: self.assertIn("service:start x-ui", commands)
                if mode=="missing_service": self.assertNotIn("service:disable x-ui", commands)
                if mode=="disabled": self.assertIn("service:disable x-ui", commands)

    def test_remote_update_download_lock_reclaims_only_abandoned_artifacts(self):
        source = (ROOT / "update.sh").read_text()
        start = source.index("acquire_update_download_lock() {")
        end = source.index("# Cheap pre-download guard.", start)
        lock_functions = source[start:end]
        with tempfile.TemporaryDirectory() as raw:
            parent = Path(raw)
            old_download = parent / ".x-ui-download.ABCDEF.tar.gz"
            new_download = parent / ".x-ui-download.GHIJKL.tar.gz"
            old_stage = parent / ".x-ui-release.MNOPQR"
            recovery = parent / "x-ui-upgrade.STUVWX"
            old_download.write_text("stale")
            new_download.write_text("recent")
            old_stage.mkdir()
            (old_stage / "staged-panel").write_text("stale stage")
            recovery.mkdir()
            (recovery / "installation").mkdir()
            (recovery / "installation" / "x-ui").write_text("recoverable")
            six_hours_ago = time.time() - (7 * 3600)
            os.utime(old_download, (six_hours_ago, six_hours_ago))
            os.utime(old_stage, (six_hours_ago, six_hours_ago))
            script = (
                "set -eu\\n" + lock_functions +
                '\\nacquire_update_download_lock "$1"\\n'
                'release_update_download_lock\\n'
            )
            result = subprocess.run(
                ["bash", "-c", script, "bash", raw], capture_output=True, text=True
            )
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse(old_download.exists())
            self.assertFalse(old_stage.exists())
            self.assertTrue(new_download.exists())
            self.assertTrue((recovery / "installation" / "x-ui").exists())

    def test_alpine_rollback_after_new_service_crashes(self):
        self.run_case("verify", alpine=True)

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
    def test_repaired_missing_service_keeps_installer_autostart(self): self.run_case("missing_service")
    def test_disabled_autostart_survives_upgrade(self): self.run_case("disabled")
    def test_rollback_keeps_originally_stopped_panel_stopped(self): self.run_case("stopped_failure")
    def test_repair_rollback_without_registered_unit(self): self.run_case("missing_unit_failure")
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

    def test_abandoned_release_staging_pruned_only_under_lock(self):
        with tempfile.TemporaryDirectory() as raw:
            base = Path(raw)
            old = base / ".x-ui-release.ABC123"
            recent = base / ".x-ui-release.NEW123"
            backup = base / "x-ui-upgrade.REC123"
            foreign = base / ".x-ui-release-not-ours"
            for folder in (old, recent, backup, foreign):
                folder.mkdir()
                (folder / "payload").write_text("keep")
            # Staging older than an hour has no active transaction once we
            # have the upgrade lock. Recovery copies must always survive.
            old_time = (base / "payload_timestamp")
            old_time.write_text("reference")
            timestamp = old.stat().st_mtime - 7200
            os.utime(old, (timestamp, timestamp))
            (base / ".x-ui-release.SYM123").symlink_to(old, target_is_directory=True)
            self.run_script(base, r'''
xui_folder="$TEST_ROOT/x-ui"
# Without an upgrade lock even abandoned staging is not eligible.
core_migration_prune_stale_stages
[[ -d "$TEST_ROOT/.x-ui-release.ABC123" ]]
exec {cm_lock}>"$TEST_ROOT/.x-ui-upgrade.lock"
flock -n "$cm_lock"
core_migration_prune_stale_stages
[[ ! -e "$TEST_ROOT/.x-ui-release.ABC123" ]]
[[ -d "$TEST_ROOT/.x-ui-release.NEW123" ]]
[[ -d "$TEST_ROOT/x-ui-upgrade.REC123" ]]
[[ -d "$TEST_ROOT/.x-ui-release-not-ours" ]]
[[ -L "$TEST_ROOT/.x-ui-release.SYM123" ]]
flock -u "$cm_lock"
exec {cm_lock}>&-
''')

    def test_verified_update_prunes_old_complete_recoveries_but_keeps_latest(self):
        with tempfile.TemporaryDirectory() as raw:
            base = Path(raw)
            backup_names = ("AAA111", "BBB222", "CCC333")
            for index, name in enumerate(backup_names):
                folder = base / ("x-ui-upgrade." + name)
                (folder / "installation").mkdir(parents=True)
                (folder / "installation" / "x-ui").write_text("old-panel")
                (folder / "data").mkdir()
                (folder / "data" / "database").write_text("recovery-data")
                stamp = 1000000000 + index * 100
                os.utime(folder, (stamp, stamp))
            unknown = base / "x-ui-upgrade.OTHER1"
            unknown.mkdir()
            (unknown / "private-data").write_text("do-not-touch")
            incomplete = base / "x-ui-upgrade.FAILED"
            (incomplete / "installation").mkdir(parents=True)
            (incomplete / "installation" / "x-ui").write_text("partial")
            (base / "x-ui-upgrade.SYM123").symlink_to(
                base / "x-ui-upgrade.AAA111", target_is_directory=True
            )
            current = base / "x-ui-upgrade.CURRENT"
            current.mkdir()
            self.run_script(base, r'''
xui_folder="$TEST_ROOT/x-ui"
cm_backup="$TEST_ROOT/x-ui-upgrade.CURRENT"
# No lock: verified snapshots may be pruned, but unmarked backup history stays.
core_migration_prune_recovery_history
[[ -d "$TEST_ROOT/x-ui-upgrade.AAA111" ]]
exec {cm_lock}>"$TEST_ROOT/.x-ui-upgrade.lock"
flock -n "$cm_lock"
core_migration_prune_snapshots
[[ ! -e "$TEST_ROOT/x-ui-upgrade.CURRENT" ]]
[[ ! -e "$TEST_ROOT/x-ui-upgrade.AAA111" ]]
[[ ! -e "$TEST_ROOT/x-ui-upgrade.BBB222" ]]
[[ -f "$TEST_ROOT/x-ui-upgrade.CCC333/data/database" ]]
[[ -f "$TEST_ROOT/x-ui-upgrade.OTHER1/private-data" ]]
[[ -f "$TEST_ROOT/x-ui-upgrade.FAILED/installation/x-ui" ]]
[[ -L "$TEST_ROOT/x-ui-upgrade.SYM123" ]]
flock -u "$cm_lock"
exec {cm_lock}>&-
''')

    def test_success_reclaims_recent_staging_and_old_downloads_only(self):
        with tempfile.TemporaryDirectory() as raw:
            base = Path(raw)
            for name in ("OLD123", "LIVE12", "SYM123"):
                folder = base / (".x-ui-release." + name)
                if name == "SYM123":
                    folder.symlink_to(base / ".x-ui-release.OLD123", target_is_directory=True)
                else:
                    folder.mkdir()
                    (folder / "x-ui").write_text("staged")
            for name in (
                ".x-ui-download.OLD123.tar.gz",
                ".x-ui-download.OLD123.tar.gz.sha256",
                ".core-migration.OLD123",
                ".x-ui-download.NEW123.tar.gz",
            ):
                (base / name).write_text("temporary")
            for name in (".x-ui-download.OLD123.tar.gz",
                         ".x-ui-download.OLD123.tar.gz.sha256",
                         ".core-migration.OLD123"):
                old = base / name
                stamp = old.stat().st_mtime - 7200
                os.utime(old, (stamp, stamp))
            (base / ".core-migration.SYM123").symlink_to(base / ".core-migration.OLD123")
            (base / "my-release.tar.gz").write_text("unrelated")
            self.run_script(base, r'''
xui_folder="$TEST_ROOT/x-ui"
cm_stage="$TEST_ROOT/.x-ui-release.LIVE12"
exec {cm_lock}>"$TEST_ROOT/.x-ui-upgrade.lock"
flock -n "$cm_lock"
core_migration_prune_stale_stages verified
core_migration_prune_stale_downloads
[[ ! -e "$TEST_ROOT/.x-ui-release.OLD123" ]]
[[ -d "$cm_stage" ]]
[[ -L "$TEST_ROOT/.x-ui-release.SYM123" ]]
[[ ! -e "$TEST_ROOT/.x-ui-download.OLD123.tar.gz" ]]
[[ ! -e "$TEST_ROOT/.x-ui-download.OLD123.tar.gz.sha256" ]]
[[ ! -e "$TEST_ROOT/.core-migration.OLD123" ]]
[[ -f "$TEST_ROOT/.x-ui-download.NEW123.tar.gz" ]]
[[ -f "$TEST_ROOT/my-release.tar.gz" ]]
[[ -L "$TEST_ROOT/.core-migration.SYM123" ]]
flock -u "$cm_lock"
exec {cm_lock}>&-
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

    def test_exit_releases_lock_inherited_by_started_panel(self):
        import signal
        for code in (0, 1):
            with self.subTest(exit_code=code), tempfile.TemporaryDirectory() as raw:
                base = Path(raw)
                try:
                    self.run_script(base, r'''
exec {cm_lock}>"$TEST_ROOT/upgrade.lock"
flock -n "$cm_lock"
sleep 60 </dev/null >/dev/null 2>&1 &
echo "$!" > "$TEST_ROOT/child.pid"
core_migration_exit ''' + str(code) + "\n")
                    result = subprocess.run(["flock", "-n", str(base / "upgrade.lock"), "true"])
                    self.assertEqual(result.returncode, 0, "started panel retained the upgrade lock")
                finally:
                    pid = base / "child.pid"
                    if pid.exists():
                        try:
                            os.kill(int(pid.read_text()), signal.SIGTERM)
                        except ProcessLookupError:
                            pass

    def test_corrupt_archive_rejected_by_space_check(self):
        with tempfile.TemporaryDirectory() as raw:
            base = Path(raw)
            (base / "release.tar.gz").write_bytes(b"invalid-gzip")
            self.run_script(base, r'''
xui_folder="$TEST_ROOT/x-ui"
if core_migration_check_space "$TEST_ROOT/release.tar.gz"; then exit 21; fi
''')

class UpgradeInterruptionTests(unittest.TestCase):
    run_script = UpgradeStorageTests.run_script

    def test_interruption_after_stop_before_snapshot_restarts_original_panel(self):
        for release in ("debian", "alpine"):
            with self.subTest(release=release), tempfile.TemporaryDirectory() as raw:
                base = Path(raw)
                (base / "stage").mkdir()
                self.run_script(base, r'''
xui_folder="$TEST_ROOT/x-ui"
cm_stage="$TEST_ROOT/stage"
cm_was_running=1
cm_stopped=1
systemctl() { printf '%s\n' "$*" >> "$TEST_ROOT/calls"; }
rc-service() { printf '%s %s\n' "$2" "$1" >> "$TEST_ROOT/calls"; }
''' + f"release={release}\n" + r'''
core_migration_exit 143
grep -q '^start x-ui$' "$TEST_ROOT/calls"
[[ ! -e "$cm_stage" ]]
''')

    def test_relative_paths_are_stable_across_download_and_install_directories(self):
        with tempfile.TemporaryDirectory() as raw:
            self.run_script(Path(raw), r'''
xui_folder="$TEST_ROOT/x-ui"
XUI_BIN_FOLDER=custom-bin
XUI_DB_FOLDER=database
core_migration_normalize_paths
[[ "$XUI_BIN_FOLDER" == "$xui_folder/custom-bin" ]]
[[ "$XUI_DB_FOLDER" == "$xui_folder/database" ]]
[[ "$XUI_MAIN_FOLDER" == "$xui_folder" ]]
cd /tmp
core_migration_normalize_paths
[[ "$XUI_DB_FOLDER" == "$xui_folder/database" ]]
''')

class CoreVersionSelectionTests(unittest.TestCase):
    run_script = UpgradeStorageTests.run_script
    def test_core_selection_and_installation(self):
        for location in ['internal', 'external', 'symlink']:
            for state in ['same', 'different', 'broken', 'missing-library', 'incompatible']:
                with self.subTest(location=location, state=state), tempfile.TemporaryDirectory() as raw:
                    base = Path(raw)
                    old = base / 'x-ui' / 'bin'
                    incoming = base / 'stage' / 'x-ui' / 'bin'
                    old.mkdir(parents=True)
                    incoming.mkdir(parents=True)
                    if location != 'internal':
                        old.rmdir()
                        old = base / 'external-bin'
                        old.mkdir()
                        if location == 'symlink':
                            (base / 'x-ui' / 'bin').symlink_to('../external-bin')
                    binary = 'hiddify-core-linux-amd64'
                    for folder, version, marker in [(old, 'v1', 'old'), (incoming, 'v2' if state == 'different' else 'v1', 'new')]:
                        code = '#!/bin/bash\n'
                        code += 'exit 1\n' if state == 'broken' and folder == old else f'echo "hiddify-core version {version} (linux)"\n# {marker}\n'
                        (folder / binary).write_text(code)
                        (folder / binary).chmod(0o755)
                        (folder / 'target.json').write_text(marker + '-manifest')
                        if not (state == 'missing-library' and folder == old):
                            (folder / 'libcronet.so').write_text(marker + '-library')
                    panel = incoming.parent / 'x-ui'
                    panel.write_text('#!/bin/bash\n' + ('[[ "$XUI_BIN_FOLDER" == */stage/x-ui/bin ]]\n' if state == 'incompatible' else 'exit 0\n'))
                    panel.chmod(0o755)
                    self.run_script(base, r'''
xui_folder="$TEST_ROOT/x-ui"
cm_stage="$TEST_ROOT/stage"
arch() { printf amd64; }
if [[ -d "$TEST_ROOT/external-bin" && ! -L "$xui_folder/bin" ]]; then
    XUI_BIN_FOLDER="$TEST_ROOT/external-bin"
fi
core_migration_select_core
if [[ "$(cat "$cm_stage/x-ui/bin/target.json")" == old-manifest ]]; then
    [[ "$cm_reuse_core" == 1 ]]
    [[ ${#cm_core_tar_args[@]} == 5 ]]
    # Simulate removal of the installation during install.sh.
    if [[ ! -L "$xui_folder/bin" && ! -d "$TEST_ROOT/external-bin" ]]; then
        rm -rf "$xui_folder/bin"
    fi
    core_migration_restore_selected_core
    core_migration_install_managed_bin
    [[ "$(cat "$XUI_BIN_FOLDER/libcronet.so")" == old-library ]]
    [[ "$(cat "$XUI_BIN_FOLDER/target.json")" == old-manifest ]]
    grep -q '# old' "$XUI_BIN_FOLDER/hiddify-core-linux-amd64"
else
    [[ "$cm_reuse_core" == 0 ]]
    [[ ${#cm_core_tar_args[@]} == 0 ]]
    [[ "$(cat "$cm_stage/x-ui/bin/target.json")" == new-manifest ]]
fi
''')

if __name__ == "__main__": unittest.main()

