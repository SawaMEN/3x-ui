#!/usr/bin/env python3
"""Build the pinned Mieru/Sudoku servers with the panel's shared speed broker."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent


def run(*args, cwd=None, env=None):
    subprocess.run(args, cwd=cwd, env=env, check=True)


def build(output, names):
    lock = json.loads((ROOT / 'deploy/speed/components.lock.json').read_text())
    output.mkdir(parents=True, exist_ok=True)
    records = {}
    for name in names:
        entry = lock[name]
        with tempfile.TemporaryDirectory(prefix='xui-speed-build-') as tmp:
            source = Path(tmp)
            run('git', 'init', '-q', str(source))
            run('git', 'remote', 'add', 'origin', entry['repository'], cwd=source)
            run('git', 'fetch', '--depth=1', 'origin', entry['commit'], cwd=source)
            run('git', 'checkout', '-q', '--detach', 'FETCH_HEAD', cwd=source)
            patch = ROOT / 'deploy/speed' / (name + '.patch')
            run('git', 'apply', '--check', str(patch), cwd=source)
            run('git', 'apply', str(patch), cwd=source)
            sdk = source / entry['sdk']
            sdk.mkdir(parents=True)
            shutil.copyfile(ROOT / 'internal/clientlimit/brokerclient/client.go', sdk / 'client.go')
            filename = 'mita' if name == 'mieru' else 'sudoku-tunnel'
            if os.environ.get('GOOS') == 'windows':
                filename += '.exe'
            env = dict(os.environ, CGO_ENABLED='0')
            run('go', 'build', '-trimpath', '-o', str(output / filename), entry['command'], cwd=source, env=env)
            shutil.copyfile(source / 'LICENSE', output / (name + '.LICENSE'))
            records[name] = {
                'source': entry,
                'patch_sha256': hashlib.sha256(patch.read_bytes()).hexdigest(),
                'sdk_sha256': hashlib.sha256((sdk / 'client.go').read_bytes()).hexdigest(),
                'binary_sha256': hashlib.sha256((output / filename).read_bytes()).hexdigest(),
            }
    (output / 'speed-components.json').write_text(json.dumps(records, indent=2) + '\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--component', choices=['mieru', 'sudoku'], action='append')
    args = parser.parse_args()
    build(args.output.resolve(), args.component or ['mieru', 'sudoku'])
