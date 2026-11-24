#!/usr/bin/env python3
"""Cleanup fixtures only; never invoke an installer or touch host update paths."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


def function(file, name):
    source = (ROOT / file).read_text()
    start = source.index(name + "() {")
    return source[start:source.index("\n}\n", start) + 3]


class UpdateCleanupTests(unittest.TestCase):
    def test_install_and_update_share_cleanup_and_locks(self):
        for name in ("cleanup_update_artifacts", "acquire_update_download_lock"):
            self.assertEqual(function("install.sh", name), function("update.sh", name))
        for file, entry in (("install.sh", "install_x-ui"), ("update.sh", "update_x-ui")):
            body = function(file, entry)
            self.assertLess(body.index("acquire_update_download_lock"), body.index("archive=$(mktemp"))

    def test_fresh_orphans_removed_but_data_recovery_and_symlinks_survive(self):
        with tempfile.TemporaryDirectory(prefix="xui cleanup $ spaces ") as raw:
            base = Path(raw)
            root = base / "x-ui"
            (root / "bin").mkdir(parents=True)
            (root / "database.db").write_text("users")
            (root / "bin" / "custom-cert.pem").write_text("certificate")
            (root / "bin" / "hiddify-core-linux-amd64").write_text("running core")
            victims = [base / ".x-ui-download.ABC123.tar.gz",
                       base / ".x-ui-download.ABC123.tar.gz.part",
                       base / ".x-ui-download.ABC123.tar.gz.sha256",
                       base / "x-ui-linux-amd64.tar.gz.part"]
            for victim in victims:
                victim.write_bytes(b"x" * 8192)
            stages = [base / ".x-ui-release.ABC123", root / ".core-stage.ABC12345"]
            for stage in stages:
                stage.mkdir()
                (stage / "payload").write_bytes(b"x" * 8192)
            failed = base / "x-ui-upgrade.failed"
            failed.mkdir()
            (failed / "data").write_text("recovery")
            verified = base / "x-ui-upgrade.verified"
            verified.mkdir()
            (verified / ".x-ui-upgrade-verified").write_text("x-ui verified upgrade v1\n")
            outside = base / "outside"
            outside.mkdir()
            (outside / "keep").write_text("never remove")
            link = base / ".x-ui-release.LINK12"
            link.symlink_to(outside, target_is_directory=True)
            fake_marker = base / "x-ui-upgrade.marker-link"
            fake_marker.mkdir()
            (fake_marker / ".x-ui-upgrade-verified").symlink_to(verified / ".x-ui-upgrade-verified")
            script = "set -eu\n" + function("update.sh", "cleanup_update_artifacts")
            script += function("update.sh", "acquire_update_download_lock")
            script += '\nacquire_update_download_lock "$TEST_ROOT"\n'
            result = subprocess.run(["bash", "-c", script], env=dict(os.environ, TEST_ROOT=raw),
                                    capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertTrue(all(not x.exists() for x in victims + stages + [verified]))
            for kept in [failed / "data", fake_marker, outside / "keep", link,
                         root / "database.db", root / "bin" / "custom-cert.pem",
                         root / "bin" / "hiddify-core-linux-amd64"]:
                self.assertTrue(kept.exists(), kept)
            self.assertIn("removed 7 artifacts", result.stdout)
            self.assertIn("free space", result.stdout)


    def test_failed_upgrade_lock_releases_download_lock(self):
        with tempfile.TemporaryDirectory() as raw:
            script = 'set -eu\n'
            script += function("install.sh", "acquire_update_download_lock")
            script += r'''
exec 8>"$TEST_ROOT/.x-ui-upgrade.lock"
flock -n 8
if acquire_update_download_lock "$TEST_ROOT"; then exit 2; fi
[[ -z "${xui_download_lock_fd:-}" ]]
exec 7>"$TEST_ROOT/.x-ui-download.lock"
flock -n 7
'''
            result = subprocess.run(["bash", "-c", script], env=dict(os.environ, TEST_ROOT=raw),
                                    capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_core_checksum_must_match_actual_binary(self):
        import hashlib
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            binary = root / "bin"
            binary.mkdir()
            name = "hiddify-core-linux-amd64"
            (binary / name).write_text("wrong")
            (binary / "unrelated").write_text("verified")
            (binary / "libcronet.so").write_text("library")
            (binary / "target.json").write_text("{}")
            for asset, source in ((name, "unrelated"), ("libcronet.so", "libcronet.so")):
                digest = hashlib.sha256((binary / source).read_bytes()).hexdigest()
                (binary / (asset + ".sha256")).write_text(digest + "  " + source + "\n")
            panel = root / "x-ui"
            panel.write_text("#!/bin/sh\nexit 0\n")
            panel.chmod(0o700)
            script = 'set -eu\nroot="$TEST_ROOT"\nbin="$root/bin"\nbinary=hiddify-core-linux-amd64\nassets=("$binary" "$binary.sha256" libcronet.so libcronet.so.sha256 target.json)\n'
            script += function("scripts/runtime/hiddify-core-update.sh", "core_update_matches")
            script += "\ncore_update_matches\n"
            result = subprocess.run(["bash", "-c", script], env=dict(os.environ, TEST_ROOT=raw),
                                    capture_output=True, text=True, timeout=10)
            self.assertNotEqual(result.returncode, 0)

    def test_core_cleanup_preserves_backups_and_running_assets(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            binary = root / "bin"
            binary.mkdir()
            stage = root / ".core-stage.ABC12345"
            stage.mkdir()
            (stage / "download.part").write_text("partial")
            backup = root / ".core-backup.ABC12345"
            backup.mkdir()
            (backup / "old-core").write_text("recovery")
            asset = binary / "hiddify-core-linux-amd64"
            asset.write_text("working")
            temp = binary / ".core-update-hiddify-core-linux-amd64.1234"
            temp.write_text("partial")
            unknown = binary / ".core-update-hiddify-core-linux-amd64.custom"
            unknown.write_text("keep")
            script = 'set -eu\nroot="$TEST_ROOT"\nbin="$root/bin"\nassets=(hiddify-core-linux-amd64 libcronet.so target.json)\n'
            script += function("scripts/runtime/hiddify-core-update.sh", "cleanup_core_update_artifacts")
            script += '\ncleanup_core_update_artifacts\n'
            result = subprocess.run(["bash", "-c", script], env=dict(os.environ, TEST_ROOT=raw),
                                    capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse(stage.exists())
            self.assertFalse(temp.exists())
            self.assertTrue((backup / "old-core").exists())
            self.assertEqual(asset.read_text(), "working")
            self.assertTrue(unknown.exists())
            self.assertIn("removed 2 artifacts", result.stdout)

    def test_core_update_checks_identity_even_when_tag_matches(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            binary = root / "bin"
            binary.mkdir()
            panel = root / "x-ui"
            panel.write_text('#!/bin/bash\n[[ "$*" == "core-migrate --check-binary" && "$XUI_BIN_FOLDER" == "$TEST_ROOT/bin" ]] && grep -qx valid "$XUI_BIN_FOLDER/target.json"\n')
            panel.chmod(0o700)
            name = "hiddify-core-linux-amd64"
            import hashlib
            for asset in (name, "libcronet.so"):
                (binary / asset).write_text(asset)
                digest = hashlib.sha256((binary / asset).read_bytes()).hexdigest()
                (binary / (asset + ".sha256")).write_text(digest + "  " + asset + "\n")
            manifest = binary / "target.json"
            manifest.write_text("valid\n")
            script = 'set -eu\nroot="$TEST_ROOT"\nbin="$root/bin"\nbinary=hiddify-core-linux-amd64\nassets=("$binary" "$binary.sha256" libcronet.so libcronet.so.sha256 target.json)\n'
            script += function("scripts/runtime/hiddify-core-update.sh", "core_update_matches")
            script += '\ncore_update_matches\n'
            def probe():
                return subprocess.run(["bash", "-c", script], env=dict(os.environ, TEST_ROOT=raw),
                                      capture_output=True, text=True, timeout=10)
            self.assertEqual(probe().returncode, 0)
            manifest.write_text("stale\n")
            self.assertNotEqual(probe().returncode, 0)
            manifest.unlink()
            self.assertNotEqual(probe().returncode, 0)
            manifest.write_text("valid\n")
            (binary / name).write_text("different binary")
            self.assertNotEqual(probe().returncode, 0)

    def test_manifest_repair_requires_identical_verified_executable(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            binary = root / "bin"
            stage = root / "stage"
            binary.mkdir()
            stage.mkdir()
            name = "hiddify-core-linux-amd64"
            (stage / name).write_text("verified")
            (binary / name).write_text("verified")
            script = 'set -eu\nbin="$TEST_ROOT/bin"\nbinary=hiddify-core-linux-amd64\n'
            script += function("scripts/runtime/hiddify-core-update.sh", "core_update_can_repair_manifest")
            script += '\ncore_update_can_repair_manifest "$TEST_ROOT/stage"\n'
            def probe():
                return subprocess.run(["bash", "-c", script], env=dict(os.environ, TEST_ROOT=raw),
                                      capture_output=True, text=True, timeout=10)
            self.assertEqual(probe().returncode, 0)
            (binary / name).write_text("wrong")
            self.assertNotEqual(probe().returncode, 0)
            (binary / name).unlink()
            (binary / name).symlink_to(stage / name)
            self.assertNotEqual(probe().returncode, 0)

    def test_stale_manifest_does_not_block_identical_core_repair(self):
        source = (ROOT / "scripts/runtime/hiddify-core-update.sh").read_text()
        # Run the actual compatibility branch, without network or systemctl.
        branch = source[source.index('if core_update_can_repair_manifest "$stage"; then'):
                        source.index("# Check the candidate with the installed panel")]
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            binary = root / "bin"
            stage = root / "stage"
            binary.mkdir()
            stage.mkdir()
            name = "hiddify-core-linux-amd64"
            (stage / name).write_text("verified")
            (binary / name).write_text("verified")
            (binary / "target.json").write_text('{"target":{"version":"sawamen-970842c884a1-062192bbe5c3-mask1-ciphers1-net62-sec1-stream1"}}')
            script = 'set -eu\nbin="$TEST_ROOT/bin"\nstage="$TEST_ROOT/stage"\nbinary=hiddify-core-linux-amd64\ncurrent_tag=sawamen-970842c884a1-062192bbe5c3-mask1-ciphers1-net62-sec1-stream1\n'
            script += function("scripts/runtime/hiddify-core-update.sh", "core_update_can_repair_manifest")
            script += "\n" + branch
            def probe():
                return subprocess.run(["bash", "-c", script], env=dict(os.environ, TEST_ROOT=raw),
                                      capture_output=True, text=True, timeout=10)
            self.assertEqual(probe().returncode, 0)
            (binary / name).write_text("different")
            self.assertNotEqual(probe().returncode, 0)

    def test_install_exit_explicitly_unlocks_inherited_descriptor(self):
        with tempfile.TemporaryDirectory() as raw:
            base = Path(raw)
            for entry in ("install_x-ui", "prepare_release_migration"):
                body = function("install.sh", entry)
                trap = next(line.strip() for line in body.splitlines()
                            if "trap 'code=$?;" in line and line.rstrip().endswith("EXIT"))
                script = 'set -eu\nxui_download_archive="$TEST_ROOT/archive"\n'
                script += 'exec {xui_download_lock_fd}>"$TEST_ROOT/lock"\nflock -n "$xui_download_lock_fd"\n'
                script += 'core_migration_exit() { return 0; }\n' + trap + '\nsleep 1 &\nexit 0\n'
                result = subprocess.run(["bash", "-c", script], env=dict(os.environ, TEST_ROOT=raw),
                                        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5)
                self.assertEqual(result.returncode, 0, result.stderr)
                probe = subprocess.run(["flock", "-n", str(base / "lock"), "true"], timeout=3)
                self.assertEqual(probe.returncode, 0, "inherited lock remained held")


if __name__ == "__main__":
    unittest.main()
