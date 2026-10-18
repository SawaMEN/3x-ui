"""Regression tests for unmodified official Telemt artifacts and update failures."""
import hashlib
import importlib.util
import io
import os
from pathlib import Path
import re
import shlex
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("fetch_official_telemt", ROOT / "tools/telemt/fetch.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class OfficialTelemtTests(unittest.TestCase):
    def release(self):
        names = list(module.ASSETS.values())
        return {"tag_name": "3.5.14", "published_at": "2026-10-05", "draft": False,
                "prerelease": False, "assets": [
                    {"name": n, "browser_download_url": module.PREFIX + "3.5.14/" + n}
                    for filename in names for n in (filename, filename + ".sha256")]}

    def test_resolve_only_stable_complete_official_releases(self):
        import json
        item = self.release()
        with patch.object(module, "fetch", side_effect=lambda *args: json.dumps(item).encode()):
            self.assertEqual(module.resolve()["tag_name"], "3.5.14")
            item["assets"].pop()
            with self.assertRaisesRegex(ValueError, "lacks"):
                module.resolve()
            item["assets"] = self.release()["assets"]
            item["tag_name"] = "3.5.14+3xui.1"
            with self.assertRaisesRegex(ValueError, "version"):
                module.resolve()

    def test_checksum_and_strict_tar_layout(self):
        with tempfile.TemporaryDirectory() as location:
            folder = Path(location)
            archive = folder / module.ASSETS["amd64"]
            with tarfile.open(archive, "w:gz") as tar:
                binary = b"\x7fELF\x02\x01" + b"\0" * 12 + b">\0"
                file = tarfile.TarInfo("telemt")
                file.size = len(binary)
                tar.addfile(file, io.BytesIO(binary))
            digest = hashlib.sha256(archive.read_bytes()).hexdigest()
            sidecar = folder / (archive.name + ".sha256")
            sidecar.write_text(digest + "  " + archive.name + "\n")
            module.verify(archive, sidecar, "sha256:" + digest)
            sidecar.write_text("0" * 64 + "  " + archive.name + "\n")
            with self.assertRaisesRegex(ValueError, "mismatch"):
                module.verify(archive, sidecar)
            sidecar.write_text(digest + "  other.tar.gz\n")
            with self.assertRaisesRegex(ValueError, "sidecar"):
                module.verify(archive, sidecar)

    def test_install_checks_architecture_and_rejects_unsafe_archives(self):
        item = self.release()
        for member_name in ("telemt", "../telemt"):
            with self.subTest(member=member_name), tempfile.TemporaryDirectory() as tmp:
                buf = io.BytesIO()
                binary = bytearray(20)
                binary[:6] = b"\x7fELF\x02\x01"
                binary[18:20] = (62).to_bytes(2, "little")
                with tarfile.open(fileobj=buf, mode="w:gz") as tar:
                    info = tarfile.TarInfo(member_name)
                    info.size = len(binary)
                    tar.addfile(info, io.BytesIO(binary))
                payload = buf.getvalue()
                archive_name = module.ASSETS["amd64"]
                hash_value = hashlib.sha256(payload).hexdigest()
                def fake_fetch(url, path):
                    data = payload if url.endswith(archive_name) else (hash_value + "  " + archive_name + "\n").encode()
                    Path(path).write_bytes(data)
                with patch.object(module, "fetch", side_effect=fake_fetch), patch.object(module.platform, "machine", return_value="unknown"):
                    if member_name != "telemt":
                        with self.assertRaisesRegex(ValueError, "unsafe"):
                            module.install(item, "amd64", Path(tmp))
                    else:
                        module.install(item, "amd64", Path(tmp))
                        self.assertEqual((Path(tmp) / "telemt").read_bytes(), bytes(binary))

class TelemtUpdaterTests(unittest.TestCase):
    def test_bad_checksum_or_binary_version_preserves_running_proxy(self):
        for failure in ("checksum", "version"):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as path:
                root = Path(path)
                served = root / "served"
                served.mkdir()
                asset = served / "telemt-x86_64-linux-musl.tar.gz"
                with tarfile.open(asset, "w:gz") as tar:
                    payload = b"#!/bin/sh\necho telemt 3.5.13\n"
                    info = tarfile.TarInfo("telemt")
                    info.size = len(payload)
                    tar.addfile(info, io.BytesIO(payload))
                digest = hashlib.sha256(asset.read_bytes()).hexdigest()
                (served / (asset.name + ".sha256")).write_text((("0" * 64) if failure == "checksum" else digest) + "  " + asset.name + "\n")
                old_binary = root / "telemt"
                old = "#!/bin/sh\necho telemt 3.5.14+3xui.1\n"
                old_binary.write_text(old)
                old_binary.chmod(0o755)
                version = root / "version"
                version.write_text("3.5.14+3xui.1\n")
                source = (ROOT / "internal/Telemt/telemt-update.sh").read_text()
                for key, value in (("BIN", old_binary), ("VERSION_FILE", version), ("TMP_DIR", root / "download")):
                    source = re.sub("^" + key + "=.*$", key + "=" + shlex.quote(str(value)), source, flags=re.M)
                source = source.replace('[[ "$EUID" -eq 0 ]]', "true")
                source = source.replace("/run/lock/x-ui-telemt-update.lock", str(root / "lock"))
                script = root / "update.sh"
                script.write_text(source)
                bin = root / "stubs"
                bin.mkdir()
                (bin / "curl").write_text("""#!/bin/bash
set -eu
output=''
while (( $# )); do
 if [[ "$1" == -o ]]; then output="$2"; shift 2; else url="$1"; shift; fi
done
if [[ -n "$output" ]]; then cp "$SERVED/${url##*/}" "$output"; else printf '  "tag_name": "3.5.14",\\n'; fi
""")
                (bin / "curl").chmod(0o755)
                (bin / "uname").write_text("#!/bin/sh\necho x86_64\n")
                (bin / "uname").chmod(0o755)
                (bin / "systemctl").write_text('#!/bin/sh\necho "$*" >> "$SERVICE_LOG"\n')
                (bin / "systemctl").chmod(0o755)
                result = subprocess.run(["bash", str(script)], capture_output=True, text=True,
                    env=dict(os.environ, PATH=str(bin) + ":" + os.environ["PATH"],
                             SERVED=str(served), SERVICE_LOG=str(root / "service-log")))
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                if failure == "checksum":
                    self.assertIn("FAILED", result.stdout + result.stderr)
                else:
                    self.assertIn("binary version mismatch", result.stderr)
                self.assertEqual(old_binary.read_text(), old)
                self.assertFalse((root / "service-log").exists(), result.stdout + result.stderr)


    def test_install_upgrade_and_no_downgrade(self):
        """The panel updater installs when absent, upgrades old builds, and leaves newer builds alone."""
        cases = [
            ("fresh", None, True),
            ("upgrade", "3.5.13", True),
            ("legacy-custom", "3.5.14+3xui.1", True),
            ("up-to-date", "3.5.14", False),
            ("newer", "3.6.0", False),
        ]
        for name, installed, changes in cases:
            with self.subTest(case=name), tempfile.TemporaryDirectory() as path:
                root = Path(path)
                served = root / "served"
                served.mkdir()
                asset = served / "telemt-x86_64-linux-musl.tar.gz"
                binary = b"#!/bin/sh\necho Telemt 3.5.14\n"
                with tarfile.open(asset, "w:gz") as tar:
                    info = tarfile.TarInfo("telemt")
                    info.size = len(binary)
                    tar.addfile(info, io.BytesIO(binary))
                (served / (asset.name + ".sha256")).write_text(
                    hashlib.sha256(asset.read_bytes()).hexdigest() + "  " + asset.name + "\n"
                )
                old_binary = root / "telemt"
                original = ("#!/bin/sh\necho Telemt " + installed + "\n").encode() if installed else None
                if original:
                    old_binary.write_bytes(original)
                    old_binary.chmod(0o755)
                version = root / "version"
                if installed:
                    version.write_text(installed + "\n")
                script = (ROOT / "internal/Telemt/telemt-update.sh").read_text()
                for key, value in (
                    ("BIN", old_binary), ("VERSION_FILE", version), ("TMP_DIR", root / "download")
                ):
                    script = re.sub("^" + key + "=.*$", key + "=" + shlex.quote(str(value)), script, flags=re.M)
                script = script.replace('[[ "$EUID" -eq 0 ]]', "true")
                script = script.replace("/run/lock/x-ui-telemt-update.lock", str(root / "lock"))
                path_script = root / "update.sh"
                path_script.write_text(script)
                stubs = root / "stubs"
                stubs.mkdir()
                commands = {
                    "curl": """#!/bin/bash
set -eu
output=''
while (( $# )); do
 if [[ "$1" == -o ]]; then output="$2"; shift 2; else url="$1"; shift; fi
done
if [[ -n "$output" ]]; then cp "$SERVED/${url##*/}" "$output"; else printf '  "tag_name": "3.5.14",\\n'; fi
""",
                    "uname": "#!/bin/sh\necho x86_64\n",
                    "systemctl": '#!/bin/sh\necho "$*" >> "$SERVICE_LOG"\nexit 1\n',
                    "pgrep": "#!/bin/sh\nexit 1\n",
                }
                for command, body in commands.items():
                    target = stubs / command
                    target.write_text(body)
                    target.chmod(0o755)
                env = dict(os.environ, PATH=str(stubs) + ":" + os.environ["PATH"],
                           SERVED=str(served), SERVICE_LOG=str(root / "service-log"))
                result = subprocess.run(["bash", str(path_script)], capture_output=True, text=True, env=env)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                if changes:
                    self.assertEqual(old_binary.read_bytes(), binary, name)
                    self.assertEqual(version.read_text().strip(), "3.5.14", name)
                else:
                    self.assertEqual(old_binary.read_bytes(), original, name)
                    self.assertEqual(version.read_text().strip(), installed, name)
                self.assertFalse((root / "download").exists(), "temporary artifacts should be cleaned")

if __name__ == "__main__":
    unittest.main()
