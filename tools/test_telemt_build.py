import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class TelemtBuildTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.upstream = self.root / 'upstream'
        self.upstream.mkdir()
        self.git('init', '-q')
        (self.upstream / 'Cargo.toml').write_text('[package]\nname = "telemt"\nversion = "3.5.14"\n')
        (self.upstream / 'Cargo.lock').write_text('[[package]]\nname = "telemt"\nversion = "3.5.14"\n')
        (self.upstream / 'fixed.txt').write_text('before\n')
        self.git('add', '.')
        self.git('-c', 'user.name=Build Test', '-c', 'user.email=test@example.invalid', 'commit', '-qm', 'base')
        commit = self.git('rev-parse', 'HEAD').strip()
        scripts = self.root / 'scripts'
        scripts.mkdir()
        shutil.copy(ROOT / 'tools/telemt/build.sh', scripts / 'build.sh')
        (scripts / 'source.json').write_text(json.dumps({'repository': str(self.upstream), 'commit': commit, 'version': '3.5.14+3xui.1'}))
        (self.upstream / 'fixed.txt').write_text('after\n')
        (scripts / '3x-ui.patch').write_text(self.git('diff'))
        self.git('restore', 'fixed.txt')
        bin_dir = self.root / 'bin'
        bin_dir.mkdir()
        cargo = bin_dir / 'cargo'
        cargo.write_text('''#!/bin/bash
set -eu
printf '%s\n' "$*" >> "$BUILD_TEST_LOG"
grep -q '3.5.14+3xui.1' Cargo.toml
grep -q '3.5.14+3xui.1' Cargo.lock
grep -q after fixed.txt
if [[ "$1" == test ]]; then exit "${FAIL_TESTS:-0}"; fi
mkdir -p target/x86_64-unknown-linux-musl/release
printf '#!/bin/sh\necho telemt v3.5.14+3xui.1\n' > target/x86_64-unknown-linux-musl/release/telemt
''')
        cargo.chmod(0o700)
        self.env = dict(os.environ, PATH=str(bin_dir) + ':' + os.environ['PATH'], BUILD_TEST_LOG=str(self.root / 'calls'))
        self.env.pop('CARGO_TARGET_DIR', None)
        self.source = self.root / 'source'
        self.output = self.root / 'output'
        self.command = ['bash', str(scripts / 'build.sh'), str(self.source), str(self.output)]

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.upstream), *args], text=True)

    def run_build(self, **env):
        return subprocess.run(self.command, env=dict(self.env, **env), capture_output=True, text=True)

    def test_build_applies_pinned_patch_tests_before_packaging_and_emits_checksum(self):
        result = self.run_build()
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = (self.root / 'calls').read_text().splitlines()
        self.assertEqual(calls, ['test --locked -- --test-threads=4', 'build --release --locked --target x86_64-unknown-linux-musl'])
        archive = self.output / 'telemt-linux-amd64.tar.gz'
        with tarfile.open(archive) as package:
            self.assertEqual(set(package.getnames()), {'telemt', 'telemt.version'})
            self.assertEqual(package.extractfile('telemt.version').read(), b'3.5.14+3xui.1\n')
        checksum = (self.output / (archive.name + '.sha256')).read_text().split()[0]
        self.assertEqual(checksum, hashlib.sha256(archive.read_bytes()).hexdigest())
        result = self.run_build()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('clean source checkout', result.stderr)
        self.assertEqual((self.source / 'fixed.txt').read_text(), 'after\n')

    def test_failed_tests_never_publish_an_archive(self):
        result = self.run_build(FAIL_TESTS='23')
        self.assertEqual(result.returncode, 23, result.stderr)
        self.assertFalse(list(self.output.glob('*.tar.gz')))
        self.assertNotIn('build --release', (self.root / 'calls').read_text())


class TelemtUpdaterTests(unittest.TestCase):
    def test_invalid_download_never_stops_running_proxy_or_replaces_binary(self):
        import io
        import re
        import shlex
        for failure in ('checksum', 'version'):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as raw:
                root = Path(raw)
                served = root / 'served'
                served.mkdir()
                tag = '3.5.14+3xui.1'
                (served / 'telemt.version').write_text(tag + '\n')
                archive = served / 'telemt-linux-amd64.tar.gz'
                with tarfile.open(archive, 'w:gz') as package:
                    for name, data in (('telemt', b'#!/bin/sh\necho telemt v3.5.14+3xui.2\n'),
                                       ('telemt.version', b'3.5.14+3xui.2\n')):
                        info = tarfile.TarInfo(name)
                        info.size = len(data)
                        info.mode = 0o755 if name == 'telemt' else 0o644
                        package.addfile(info, io.BytesIO(data))
                checksum = '0' * 64 if failure == 'checksum' else hashlib.sha256(archive.read_bytes()).hexdigest()
                (served / (archive.name + '.sha256')).write_text(checksum + '  ' + archive.name + '\n')
                binary = root / 'telemt'
                old_binary = '#!/bin/sh\necho telemt v3.5.13\n'
                binary.write_text(old_binary)
                binary.chmod(0o700)
                version = root / 'version'
                version.write_text(tag)
                source = (ROOT / 'internal/Telemt/telemt-update.sh').read_text()
                for key, value in (('BIN', binary), ('VERSION_FILE', version), ('TMP_DIR', root / 'download')):
                    source = re.sub(r'^' + key + r'=.*$', key + '=' + shlex.quote(str(value)), source, flags=re.M)
                source = source.replace('[[ "$EUID" -eq 0 ]]', 'true')
                source = source.replace("/run/lock/x-ui-telemt-update.lock", str(root / "lock"))
                helper = root / 'update.sh'
                helper.write_text(source)
                stub = root / 'stubs'
                stub.mkdir()
                curl = stub / 'curl'
                curl.write_text('''#!/bin/bash
set -eu
output=''
while (( $# )); do
  if [[ "$1" == -o ]]; then output="$2"; shift 2; else url="$1"; shift; fi
done
if [[ -n "$output" ]]; then cp "$SERVED/${url##*/}" "$output"; else cat "$SERVED/${url##*/}"; fi
''')
                curl.chmod(0o700)
                uname = stub / 'uname'
                uname.write_text('#!/bin/sh\necho x86_64\n')
                uname.chmod(0o700)
                service = stub / 'systemctl'
                service.write_text('#!/bin/sh\necho "$*" >> "$SERVICE_LOG"\n')
                service.chmod(0o700)
                result = subprocess.run(['bash', str(helper)], capture_output=True, text=True,
                                        env=dict(os.environ, PATH=str(stub) + ':' + os.environ['PATH'],
                                                 SERVED=str(served), SERVICE_LOG=str(root / 'service-log')))
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                if failure == 'checksum':
                    self.assertIn('FAILED', result.stdout)
                else:
                    self.assertIn('release changed during download', result.stderr)
                self.assertEqual(binary.read_text(), old_binary)
                self.assertFalse((root / 'service-log').exists(), result.stdout + result.stderr)


if __name__ == '__main__':
    unittest.main()
