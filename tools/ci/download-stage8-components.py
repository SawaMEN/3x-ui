#!/usr/bin/env python3
"""Fetch only checksum-pinned native components used by stage 8."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

arch, output = sys.argv[1], Path(sys.argv[2]).resolve()
manifest = json.loads(Path('docs/stage8-components.json').read_text())
output.mkdir(parents=True, exist_ok=True)
hashes = []
for name, component in manifest.items():
    asset = component['assets'][arch]
    directory = output / name
    directory.mkdir(exist_ok=True)
    destination = directory / asset['name']
    subprocess.run(['curl', '-fsSL', '--retry', '3', '--max-time', '180', asset['url'], '-o', str(destination)], check=True)
    with destination.open('rb') as source:
        digest = hashlib.file_digest(source, 'sha256').hexdigest()
    if digest != asset['sha256']:
        raise ValueError(f'{name}: SHA256 mismatch')
    hashes.append(f'{digest}  {destination}')
    if destination.name.endswith('.tar.gz'):
        # These are the exact reviewed release bytes, checked before extraction.
        subprocess.run(['tar', '-xzf', str(destination), '-C', str(directory)], check=True)
    else:
        destination.chmod(0o700)
    print(f'Verified {name} {component["tag"]} {arch}', flush=True)

def find(name, filename):
    choices = [p for p in (output/name).rglob(filename) if p.is_file()]
    if len(choices) != 1:
        raise ValueError(f'{name}: expected exactly one {filename}')
    choices[0].chmod(0o700)
    return choices[0]
vetroff = find('vetroff', 'vetroff')
settings = {
    'STAGE8_VETROFF_BUNDLE': vetroff.parent.parent,
    'VETROFF_TEST_BINARY': vetroff,
    'VETROFF_TEST_ARCHIVE': output/'vetroff'/manifest['vetroff']['assets'][arch]['name'],
    'TELEMT_TEST_BINARY': find('telemt','telemt'),
    'TRUSTTUNNEL_TEST_BINARY': find('trusttunnel','trusttunnel_endpoint'),
    'STAGE8_OPENFLUX_BINARY': find('openflux',f'openflux-linux-{arch}'),
    'SUDOKU_TEST_BINARY': find('sudoku','sudoku'),
}
with open(os.environ['GITHUB_ENV'],'a') as env:
    for key,value in settings.items():
        env.write(f'{key}={value}\n')
Path('stage8-binaries.sha256').write_text('\n'.join(hashes)+'\n')
