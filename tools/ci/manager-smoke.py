#!/usr/bin/env python3
"""Exercise manager generation against two isolated real panel/core installations."""
import argparse
import base64
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import socketserver
import sqlite3
import struct
import subprocess
import tempfile
import threading
import time
import urllib.request
import uuid


def run(*args, **kwargs):
    return subprocess.check_output(args, text=True, **kwargs)


def eventually(fn, timeout=45):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        try:
            value = fn()
            if value:
                return value
        except (OSError, ValueError, AssertionError) as exc:
            last = exc
        time.sleep(0.2)
    raise AssertionError(f"condition did not converge: {last}")


class Echo(socketserver.BaseRequestHandler):
    def handle(self):
        while data := self.request.recv(65536):
            self.request.sendall(data)


def api(base, token, path, body=None):
    req = urllib.request.Request(base + '/panel/api/' + path, headers={
        'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'},
        data=None if body is None else json.dumps(body).encode())
    with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(req, timeout=65) as response:
        result = json.load(response)
    assert result['success'], f"{path}: {result.get('msg')}"
    return result.get('obj')


def public(base, token):
    with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(base + '/bus/' + token + '?format=singbox', timeout=20) as response:
        return json.load(response)


def exact(conn, size, stage='payload'):
    out = b''
    while len(out) < size:
        part = conn.recv(size - len(out))
        assert part, f'unexpected EOF during {stage}'
        out += part
    return out


def main():
    args = argparse.ArgumentParser(description=__doc__)
    args.add_argument('--panel', required=True, type=Path)
    args.add_argument('--core-dir', required=True, type=Path)
    args = args.parse_args()
    panel = args.panel.resolve()
    core_dir = args.core_dir.resolve()
    network = 'manager-smoke-' + uuid.uuid4().hex[:12]
    containers = []
    subprocess.run(['docker', 'network', 'create', network], check=True, stdout=subprocess.DEVNULL)
    try:
        with tempfile.TemporaryDirectory(prefix='manager-smoke-') as temp:
            root = Path(temp)
            installations = []
            for role in ('master', 'node'):
                home = root / role
                (home / 'db').mkdir(parents=True)
                shutil.copytree(core_dir, home / 'bin')
                key = home / 'db/node-token-key.json'
                key.write_text(json.dumps({'active': 'smoke', 'keys': {'smoke': base64.b64encode(secrets.token_bytes(32)).decode()}}))
                key.chmod(0o600)
                env = {**os.environ, 'XUI_DB_FOLDER': str(home / 'db'), 'XUI_BIN_FOLDER': str(home / 'bin'),
                       'XUI_LOG_FOLDER': str(home / 'db'), 'NODE_TOKEN_ENCRYPTION': 'required',
                       'XUI_NODE_TOKEN_KEY_FILE': str(key), 'XUI_INIT_WEB_BASE_PATH': '/'}
                run(str(panel), 'setting', '-username', 'smoke', '-password', secrets.token_urlsafe(32),
                    '-port', '2053', '-webBasePath', '/', '-listenIP', '0.0.0.0', env=env)
                token = re.search(r'apiToken:\s*(\S+)', run(str(panel), 'setting', '-getApiToken', '-tokenScope', 'admin', env=env))[1]
                with sqlite3.connect(home / 'db/x-ui.db') as db:
                    db.execute("DELETE FROM settings WHERE key='subEnable'")
                    db.execute("INSERT INTO settings(key,value) VALUES('subEnable','false')")
                    # This isolated test deliberately proxies to the bridge echo server.
                    # Keep the production template's private-egress protection unchanged.
                    template = json.loads((Path(__file__).resolve().parents[2] / 'internal/web/service/config.json').read_text())
                    template['routing']['rules'] = [r for r in template['routing']['rules'] if 'geoip:private' not in r.get('ip', [])]
                    for outbound in template['outbounds']:
                        if outbound['protocol'] == 'freedom':
                            outbound['settings']['finalRules'] = []
                    db.execute("DELETE FROM settings WHERE key='xrayTemplateConfig'")
                    db.execute("INSERT INTO settings(key,value) VALUES('xrayTemplateConfig',?)", (json.dumps(template),))
                    # Persist the new installation's selected core alongside its template.
                    # A template without coreType is intentionally treated as a legacy DB.
                    db.execute("DELETE FROM settings WHERE key='coreType'")
                    db.execute("INSERT INTO settings(key,value) VALUES('coreType','sing-box')")
                name = network + '-' + role
                containers.append(name)
                subprocess.run(['docker', 'run', '-d', '--name', name, '--network', network, '--cap-drop', 'ALL', '--user', f'{os.getuid()}:{os.getgid()}',
                    '-p', '127.0.0.1::2053', '-v', f'{panel}:/panel:ro', '-v', f'{home}/db:/data',
                    '-v', f'{home}/bin:/runtime', '-e', 'XUI_DB_FOLDER=/data', '-e', 'XUI_BIN_FOLDER=/runtime',
                    '-e', 'XUI_LOG_FOLDER=/data', '-e', 'XUI_INIT_WEB_BASE_PATH=/', '-e', 'NODE_TOKEN_ENCRYPTION=required',
                    '-e', 'XUI_NODE_TOKEN_KEY_FILE=/data/node-token-key.json', 'ubuntu:24.04', '/panel', 'run'],
                    check=True, stdout=subprocess.DEVNULL)
                host_port = run('docker', 'port', name, '2053/tcp').strip().rsplit(':', 1)[1]
                ip = json.loads(run('docker', 'inspect', name))[0]['NetworkSettings']['Networks'][network]['IPAddress']
                base = 'http://127.0.0.1:' + host_port
                eventually(lambda: api(base, token, 'server/status'))
                installations.append([home, env, name, token, base, ip])
            master, node = installations
            node_view = api(master[4], master[3], 'nodes/add', {'name': 'smoke-node', 'scheme': 'http',
                'address': node[5], 'port': 2053, 'basePath': '/', 'apiToken': node[3], 'enable': True,
                'allowPrivateAddress': True, 'tlsVerifyMode': 'verify', 'inboundSyncMode': 'all'})
            entries = []
            for protocol, node_id, ip in [('vless', 0, master[5]), ('vmess', node_view['id'], node[5])]:
                stream = {'network': 'tcp', 'security': 'none'}
                if protocol == 'vmess':
                    stream.update(network='ws', wsSettings={'path': '/manager-smoke'})
                entries.append({'mode': 'generate', 'nodeId': node_id, 'port': 'random', 'label': protocol,
                    'config': {'protocol': protocol, 'shareAddrStrategy': 'custom', 'shareAddr': ip,
                    'settings': json.dumps({'clients': [], 'decryption': 'none'}), 'streamSettings': json.dumps(stream), 'sniffing': '{}'}})
            subscription = api(master[4], master[3], 'manager/subscriptions', {'name': 'Smoke', 'enabled': True,
                'intervalMinutes': 10, 'graceMinutes': 60, 'entries': entries})
            sid = subscription['id']
            def detail():
                return api(master[4], master[3], f'manager/subscriptions/{sid}')
            def rotate(expect='success'):
                op = api(master[4], master[3], f'manager/subscriptions/{sid}/rotate', {})
                def finished():
                    state = api(master[4], master[3], 'manager/state')
                    row = next((r for r in state['operations'] if r['id'] == op['id']), None)
                    return row if row and row['state'] not in ('queued', 'running') else None
                result = eventually(finished, 100)
                assert result['state'] == expect, result
                return detail()
            first = rotate()
            first_ids = [e['activeInboundId'] for e in first['published']]
            def traffic():
                config = public(master[4], first['token'])
                configs = config if isinstance(config, list) else [config]
                outbounds = [o for c in configs for o in c.get('outbounds', []) if o.get('type') in ('vless', 'vmess')]
                assert {o['type'] for o in outbounds} == {'vless', 'vmess'}, outbounds
                with socketserver.ThreadingTCPServer(('0.0.0.0', 0), Echo) as echo:
                    threading.Thread(target=echo.serve_forever, daemon=True).start()
                    # Docker's bridge gateway is reachable from both installations.
                    gateway = json.loads(run('docker', 'network', 'inspect', network))[0]['IPAM']['Config'][0]['Gateway']
                    for index, outbound in enumerate(outbounds):
                        with socket.socket() as reserve:
                            reserve.bind(('127.0.0.1', 0)); port = reserve.getsockname()[1]
                        print(f"Testing {outbound['type']} {outbound.get('server')}:{outbound.get('server_port')} transport={outbound.get('transport', {}).get('type', 'tcp')}", flush=True)
                        outbound = {**outbound, 'tag': 'test'}
                        cfg = {'log': {'level': 'debug'}, 'inbounds': [{'type': 'mixed', 'listen': '127.0.0.1', 'listen_port': port}],
                               'outbounds': [outbound], 'route': {'final': 'test'}}
                        path = root / f'client-{index}.json'; path.write_text(json.dumps(cfg))
                        subprocess.run([str(core_dir / 'hiddify-core-linux-amd64'), 'check', '-c', str(path)], cwd=core_dir, check=True)
                        with open(root / f'client-{index}.log', 'w') as log:
                            client = subprocess.Popen([str(core_dir / 'hiddify-core-linux-amd64'), 'srun', '-c', str(path)], cwd=core_dir, stdout=log, stderr=log)
                            try:
                                def connect():
                                    if client.poll() is not None:
                                        raise RuntimeError((root / f'client-{index}.log').read_text())
                                    conn = socket.create_connection(('127.0.0.1', port), timeout=4)
                                    conn.settimeout(5)
                                    conn.sendall(b'\x05\x01\x00'); assert exact(conn, 2, 'SOCKS negotiation') == b'\x05\x00'
                                    conn.sendall(b'\x05\x01\x00\x01' + socket.inet_aton(gateway) + struct.pack('!H', echo.server_address[1]))
                                    header = exact(conn, 4, 'SOCKS connect reply'); assert header[:2] == b'\x05\x00', header
                                    exact(conn, 4 if header[3] == 1 else 16); exact(conn, 2)
                                    payload = secrets.token_bytes(4096); conn.sendall(payload); assert exact(conn, len(payload)) == payload
                                    conn.close(); return True
                                eventually(connect)
                            except BaseException:
                                print((root / f'client-{index}.log').read_text(), flush=True)
                                raise
                            finally:
                                client.terminate(); client.wait(timeout=10)
                    echo.shutdown()
            traffic()
            second = rotate()
            assert [e['activeInboundId'] for e in second['published']] != first_ids
            with sqlite3.connect(master[0] / 'db/x-ui.db') as db:
                assert db.execute('SELECT COUNT(*) FROM inbounds WHERE id IN (?,?)', first_ids).fetchone()[0] == 2
            traffic()
            subprocess.run(['docker', 'stop', '-t', '15', node[2]], check=True, stdout=subprocess.DEVNULL)
            failed = rotate('failed')
            assert failed['published'] == second['published'], 'failed operation replaced the publication'
            subprocess.run(['docker', 'start', node[2]], check=True, stdout=subprocess.DEVNULL)
            # Docker may allocate a new host port for a dynamically published port.
            host_port = run('docker', 'port', node[2], '2053/tcp').strip().rsplit(':', 1)[1]
            node[4] = 'http://127.0.0.1:' + host_port
            eventually(lambda: api(node[4], node[3], 'server/status'))
            api(master[4], master[3], f'nodes/probe/{node_view["id"]}', {})
            rotate(); traffic()
            subprocess.run(['docker', 'stop', '-t', '15', master[2]], check=True, stdout=subprocess.DEVNULL)
            password = root / 'password'; password.write_text(secrets.token_urlsafe(32)); password.chmod(0o600)
            bundle = root / 'recovery'; restored = root / 'restored'
            run(str(panel), 'recovery', 'export', '--file', str(bundle), '--password-file', str(password), env=master[1])
            run(str(panel), 'recovery', 'check', '--file', str(bundle), '--password-file', str(password), env=master[1])
            run(str(panel), 'recovery', 'restore', '--file', str(bundle), '--password-file', str(password), '--out', str(restored), env=master[1])
            assert (restored / 'keys/node_token_key.json').read_bytes() == (master[0] / 'db/node-token-key.json').read_bytes()
            with sqlite3.connect(restored / 'database/backup.db') as db:
                assert db.execute('SELECT COUNT(*) FROM manager_subscriptions').fetchone()[0] == 1
            print('Manager live smoke passed: local VLESS, remote VMess/WS, import, echo payload, rotation, grace, node failure/recovery and encrypted restore')
    finally:
        for name in reversed(containers):
            subprocess.run(['docker', 'logs', '--tail', '40', name], check=False)
            subprocess.run(['docker', 'rm', '-f', name], check=False, stdout=subprocess.DEVNULL)
        subprocess.run(['docker', 'network', 'rm', network], check=False, stdout=subprocess.DEVNULL)


if __name__ == '__main__':
    main()
