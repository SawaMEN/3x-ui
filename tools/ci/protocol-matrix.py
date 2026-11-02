#!/usr/bin/env python3
"""Derive protocol evidence from completed native gates, never from a test plan."""
import argparse
import csv
import json
import os
import re
from pathlib import Path


def events(path):
    result = [json.loads(line) for line in path.read_text().splitlines() if line.strip()]
    failures = [e.get('Test', e.get('Package')) for e in result if e.get('Action') == 'fail']
    if failures:
        raise ValueError(f'{path}: failed checks: {failures}')
    return result


def passed(items, root):
    statuses = {e['Test']: e['Action'] for e in items
                if e.get('Test') and e.get('Action') in ('pass', 'fail', 'skip')}
    if statuses.get(root) != 'pass':
        raise ValueError(f'{root}: missing or not passed')
    bad = {k: v for k, v in statuses.items() if k.startswith(root + '/') and v != 'pass'}
    if bad:
        raise ValueError(f'{root}: skipped or failed children: {bad}')
    return statuses


def build(directory):
    rows = []
    def add(protocol, direction, core, transport, payload, status, source, test):
        rows.append(dict(protocol=protocol, direction=direction, core=core,
                         transport=transport, payload=payload, result=status,
                         evidence_file=source, evidence_test=test,
                         commit=os.environ.get('GITHUB_SHA', ''),
                         arch=os.environ.get('RUNNER_ARCH', '')))

    profiles = {
        'owner': ('owner-transfer.jsonl', ['tcp', 'raw', 'ws', 'grpc', 'http', 'httpupgrade', 'xhttp', 'kcp', 'xdrive', 'native-quic']),
        'upstream-1.14.4': ('upstream-transfer.jsonl', ['tcp', 'raw', 'ws', 'grpc', 'http', 'httpupgrade']),
        'xray-26.10.21': ('xray-classic-transfer.jsonl', ['tcp', 'raw', 'ws', 'grpc', 'httpupgrade', 'xhttp', 'kcp', 'xdrive']),
    }
    for core, (filename, networks) in profiles.items():
        items = events(directory / filename)
        passed(items, 'TestStage5ProtocolTraffic')
        for protocol in ('vless', 'vmess', 'trojan'):
            for network in networks:
                if network == 'xdrive' and protocol != 'vless':
                    continue
                root = f'TestStage5ProtocolTraffic/{protocol}/{network}'
                passed(items, root)
                for user in range(1, 5):
                    passed(items, f'{root}/user{user}')
                for direction in ('inbound', 'outbound'):
                    for payload in ('TCP', 'UDP'):
                        add(protocol, direction, core, network, payload,
                            'data-pass; IPv4/IPv6/DNS; reconnect; auth', filename, root)

    filename = 'owner-transfer.jsonl'
    items = events(directory / filename)
    for protocol in ('socks', 'http', 'shadowsocks-aead', 'shadowsocks-2022',
                     'hysteria', 'hysteria2', 'tuic', 'anytls', 'naive', 'snell5', 'snell6'):
        root = f'TestStage5ExtendedProtocolTraffic/{protocol}'
        passed(items, root)
        for flag in ('true', 'false'):
            passed(items, root + '/authorized=' + flag)
        for direction in ('inbound', 'outbound'):
            for payload in ('TCP',) if protocol in ('http', 'naive') else ('TCP', 'UDP'):
                add(protocol, direction, 'owner', 'native-default', payload,
                    'data-pass; IPv4/IPv6/DNS; reconnect; auth', filename, root)

    filename = 'database-protocols.jsonl'
    items = events(directory / filename)
    for protocol, test in (('wireguard', 'TestNativeWireGuardConnections'), ('awg', 'TestNativeAWGConnections')):
        for flag, direction in (('false', 'endpoint-initiator'), ('true', 'endpoint-responder')):
            for family in ('IPv4', 'IPv6'):
                for payload in ('tcp', 'udp'):
                    root = f'{test}/responder={flag}/{family}/{payload}'
                    passed(items, root)
                    add(protocol, direction, 'owner', 'native-netstack', payload.upper(),
                        f'data-pass; {family}; peer revocation', filename, root)
            for payload in ('tcp', 'udp'):
                passed(items, f'{test}/responder={flag}/revoked/{payload}')

    filename = 'native-schema.jsonl'
    items = events(directory / filename)
    statuses = passed(items, 'TestNativeOutboundBinaryMatrix')
    for root in statuses:
        prefix = 'TestNativeOutboundBinaryMatrix/'
        if root.startswith(prefix):
            add(root[len(prefix):], 'outbound', 'owner', 'native-options', 'none',
                'constructor-pass; not a data claim', filename, root)

    filename = 'masque-transfer.jsonl'
    items = events(directory / filename)
    passed(items, 'TestMASQUEConnections')
    for version in (1, 2, 3):
        root = f'TestMASQUEConnections/HTTP{version}'
        passed(items, root)
        for payload in ('tcp', 'udp'):
            passed(items, f'{root}/invalid-credentials/{payload}')
            for direction in ('endpoint-client', 'endpoint-server'):
                add('masque', direction, 'owner', f'HTTP{version}; no fallback', payload.upper(),
                    'data-pass; IPv4; auth', filename, root)

    filename = 'subscription-transfer.jsonl'
    items = events(directory / filename)
    root = 'TestAnyTLSShadowTLSSubscriptionConnections'
    passed(items, root)
    for payload in ('tcp', 'udp'):
        passed(items, f'{root}/{payload}')
        for layer in ('anytls', 'shadowtls'):
            passed(items, f'{root}/wrong-{layer}/{payload}')
        for direction in ('inbound', 'outbound'):
            add('anytls over shadowtls', direction, 'owner', 'generated subscription chain', payload.upper(),
                'data-pass; IPv4; both authentication layers', filename, root)

    for core, filename in (('owner', 'owner-transfer.jsonl'), ('xray-26.10.21', 'xray-transfer.jsonl')):
        items = events(directory / filename)
        passed(items, 'TestXHTTPRuntimeTransfer')
        for security in ('none', 'tls', 'reality') if core.startswith('xray') else ('none', 'tls'):
            for mode in ('auto', 'packet-up', 'stream-up', 'stream-one'):
                if security == 'none' and mode not in ('auto', 'packet-up'):
                    continue
                for split in (False, True):
                    if split and mode == 'stream-one':
                        continue
                    root = f'TestXHTTPRuntimeTransfer/{security}/{mode}' + ('/split-download' if split else '')
                    passed(items, root)
                    for direction in ('inbound', 'outbound'):
                        add('vless', direction, core, f'xhttp/{security}/{mode}' + ('/split-download' if split else ''),
                            'TCP', 'data-pass; large HTTP body; sequential requests', filename, root)

    filename = 'fragmentation-contract.jsonl'
    items = events(directory / filename)
    passed(items, 'TestStage5FragmentedTrojanContract')
    if not any('KNOWN_UPSTREAM_DEFECT:' in e.get('Output', '') for e in items):
        raise ValueError('missing explicit upstream defect evidence')
    add('trojan', 'inbound', 'upstream-1.14.4', 'TCP/TLS fragmented key', 'TCP',
        'KNOWN_UPSTREAM_DEFECT; control passed; not a successful fragmented transfer',
        filename, 'TestStage5FragmentedTrojanContract/upstream')
    add('trojan', 'inbound', 'owner', 'TCP/TLS fragmented key', 'TCP',
        'data-pass', filename, 'TestStage5FragmentedTrojanContract/owner')
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--log-dir', type=Path, default=Path('.'))
    parser.add_argument('--output', type=Path, default=Path('protocol-matrix.csv'))
    args = parser.parse_args()
    rows = build(args.log_dir)
    policy = json.loads((args.log_dir / 'capability-inbounds.json').read_text())
    models = sorted({r['protocol'] for r in policy})
    def form_catalog(direction):
        text = Path(f'frontend/src/schemas/protocols/{direction}/index.ts').read_text()
        return sorted(set(re.findall(r"protocol:\s*z.literal\('([^']+)'\)", text)))
    inbound_forms = form_catalog('inbound')
    if models != inbound_forms:
        raise ValueError(f'model/form catalog mismatch: {set(models) ^ set(inbound_forms)}')
    inventory = dict(inbound_models=models, inbound_forms=inbound_forms,
                     outbound_forms=form_catalog('outbound'), inbound_policy=policy,
                     native_outbound_constructors=sorted({r['protocol'] for r in rows
                         if r['result'].startswith('constructor-pass')}),
                     xray_module=re.search(r'github.com/xtls/xray-core (\S+)', Path('go.mod').read_text()).group(1),
                     note='Policy, forms and constructors are inventory evidence; data evidence is in the CSV.')
    args.output.with_name('protocol-inventory.json').write_text(json.dumps(inventory, indent=2) + '\n')
    with args.output.open('w', newline='') as out:
        writer = csv.DictWriter(out, fieldnames=list(rows[0]))
        writer.writeheader()
        writer.writerows(rows)
    print(f'Wrote {len(rows)} observed rows; constructor and known-defect rows are separate from data success')


if __name__ == '__main__':
    main()
