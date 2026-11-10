#!/usr/bin/env python3
"""Account for every changed path; review marks require explicit evidence."""
import argparse
import collections
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import os

ROOT = Path(__file__).resolve().parents[2]
SOURCE = 'f3baf84ab124c017812924481ccc80d6ce0be306'
BASE = '416a66a3343d446b9b18a2a8b2b0eb8771ea72fa'
STATE = ROOT / 'docs/audit/f3baf84-files.json'


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT)


def subsystem(path):
    if path.startswith(('internal/database/', 'internal/coremodel/')):
        return 'database'
    if path.startswith(('internal/sub/', 'internal/util/link/')):
        return 'subscriptions'
    if path.startswith('internal/clientlimit/') or 'client_speed' in path:
        return 'limits'
    if path.startswith('frontend/'):
        return 'frontend'
    if path.startswith(('internal/web/controller/', 'internal/web/middleware/', 'internal/web/websocket/', 'internal/domainfront/')):
        return 'api-gateway'
    if path.startswith(('internal/web/service/', 'internal/web/job/', 'internal/web/runtime/')):
        return 'services-nodes-jobs'
    if path.startswith(('internal/hiddify/', 'internal/singbox/', 'internal/xray/', 'internal/coremigration/')):
        return 'core-runtime'
    if path.startswith(('docs/', 'media/')) or path.endswith(('.md', '.mdx')):
        return 'documentation-assets'
    if path.startswith(('.github/', 'deploy/', 'tools/', 'scripts/')) or path in ('install.sh', 'update.sh', 'Makefile', 'go.mod', 'go.sum'):
        return 'installation-ci'
    return 'protocols-utilities'


def kind(path):
    if '/generated/' in path or path.endswith(('openapi.json', 'native-outbound-fields.json', 'native_import_options.json')):
        return 'generated'
    if path.endswith(('_test.go', '.test.ts', '.test.tsx')) or '/testdata/' in path:
        return 'test-fixture'
    if path.endswith(('.md', '.mdx')) or path.startswith(('docs/', 'media/')):
        return 'documentation-asset'
    if path.endswith(('lock.json', 'package-lock.json', 'go.sum')):
        return 'dependency-lock'
    return 'source-config'


def inventory():
    chunks = git('diff', '--numstat', '-z', BASE, SOURCE).decode().split('\0')
    entries = []
    i = 0
    while i < len(chunks) and chunks[i]:
        added, deleted, path = chunks[i].split('\t', 2)
        i += 1
        previous = None
        if not path:
            previous, path = chunks[i:i + 2]
            i += 2
        entries.append(dict(path=path, previous_path=previous,
                            additions=None if added == '-' else int(added),
                            deletions=None if deleted == '-' else int(deleted),
                            subsystem=subsystem(path), kind=kind(path),
                            status='pending', evidence='', reviewed_sha256=None))
    return sorted(entries, key=lambda entry: entry['path'])


def digest(path):
    item = ROOT / path
    return hashlib.sha256(item.read_bytes()).hexdigest() if item.is_file() else 'absent'


def write(state):
    STATE.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile('w', dir=STATE.parent, encoding='utf-8', delete=False) as out:
        temp = out.name
        json.dump(state, out, ensure_ascii=False, indent=2)
        out.write('\n')
    try:
        os.replace(temp, STATE)
    finally:
        if os.path.exists(temp):
            os.unlink(temp)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['init', 'report', 'mark', 'check'])
    parser.add_argument('paths', nargs='*')
    parser.add_argument('--evidence')
    args = parser.parse_args()
    expected = inventory()
    if not STATE.exists():
        if args.action != 'init':
            parser.error('run init first')
        write(dict(schema_version=1, source_commit=SOURCE, base_commit=BASE, files=expected))
    state = json.loads(STATE.read_text())
    if state['source_commit'] != SOURCE or state['base_commit'] != BASE:
        parser.error('inventory belongs to another diff')
    actual = {row['path']: row for row in state['files']}
    if len(actual) != len(expected) or set(actual) != {row['path'] for row in expected}:
        parser.error('inventory does not cover the complete original diff exactly once')
    if args.action == 'mark':
        if not args.evidence or not args.paths:
            parser.error('mark requires paths and --evidence describing the completed review')
        for path in args.paths:
            if path not in actual:
                parser.error(f'path is outside original diff: {path}')
        for path in args.paths:
            actual[path].update(status='reviewed', evidence=args.evidence, reviewed_sha256=digest(path))
        write(state)
    counts = collections.defaultdict(collections.Counter)
    for row in state['files']:
        status = row['status']
        if status == 'reviewed' and row['reviewed_sha256'] != digest(row['path']):
            status = 'stale'
        counts[row['subsystem']][status] += 1
    total = collections.Counter()
    print('| Subsystem | Total | Reviewed | Pending | Stale |')
    print('|---|---:|---:|---:|---:|')
    for name, count in sorted(counts.items()):
        total.update(count)
        print(f"| {name} | {sum(count.values())} | {count['reviewed']} | {count['pending']} | {count['stale']} |")
    print(f"| TOTAL | {sum(total.values())} | {total['reviewed']} | {total['pending']} | {total['stale']} |")
    if args.action == 'check' and total['stale']:
        raise SystemExit('reviewed files changed: review them again before closing the audit')


if __name__ == '__main__':
    main()
