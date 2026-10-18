#!/usr/bin/env python3
"""Exercise the real nftables helper inside a disposable network namespace.

Run only through: sudo unshare --net -- python3 tools/test-meko-firewall.py
No firewall command is permitted in the host network namespace.
"""
import os
from pathlib import Path
import re
import subprocess
import tempfile


def run(*args):
    result = subprocess.run(args, text=True, capture_output=True, timeout=30)
    if result.returncode:
        raise RuntimeError(f"{args}: {result.stderr}\n{result.stdout}")
    return result.stdout


def main():
    # unshare --net leaves PID 1 in the host namespace. Refuse accidental
    # direct invocation even by root; never alter the runner's own firewall.
    if os.readlink('/proc/self/ns/net') == os.readlink('/proc/1/ns/net'):
        raise RuntimeError('Refusing host namespace; run through sudo unshare --net --')
    source = Path(__file__).resolve().parents[1] / 'internal/Telemt/telemt-meko-fix.sh'
    with tempfile.TemporaryDirectory(prefix='meko-firewall-') as directory:
        root = Path(directory)
        configs = root / 'mtproto'
        configs.mkdir()
        helper = root / 'helper.sh'
        script = source.read_text().replace('/run/lock/x-ui-telemt-meko.lock', str(root / 'lock'))
        script = script.replace('/etc/x-ui/telemt-meko-fix.env', str(root / 'state.env'))
        script = script.replace('/usr/local/x-ui/bin/mtproto/telemt-*.toml', str(configs / 'telemt-*.toml'))
        helper.write_text(script)
        for key in ('TELEMT_PORTS', 'TELEMT_MEKO_ENABLED', 'TELEMT_MEKO_RATE', 'TELEMT_MEKO_BURST'):
            os.environ.pop(key, None)

        def config(port, rate, burst):
            (configs / 'telemt-1.toml').write_text(
                f'# x-ui-meko-fix=1\n# x-ui-meko-rate={rate}\n# x-ui-meko-burst={burst}\n'
                f'[server]\nport = {port}\n')

        def apply():
            run('bash', str(helper), 'apply')

        def status():
            return dict(line.split('=', 1) for line in run('bash', str(helper), 'status').splitlines() if '=' in line)

        def healthy(port):
            actual = status()
            assert actual['backend'] == 'nftables', actual
            assert actual['installed'] == 'true', actual
            for key in ('ports', 'applied_ports', 'verified_ports'):
                assert actual[key] == str(port), actual

        config(8443, 54, 1)
        apply()
        healthy(8443)
        apply()  # Replacing an existing table must remain valid and atomic.
        healthy(8443)

        config(7443, 120, 8)
        assert status()['installed'] == 'false', 'Old port incorrectly verified'
        apply()
        healthy(7443)

        config(7443, 240, 4)
        assert status()['installed'] == 'false', 'Old limits incorrectly verified'
        apply()
        healthy(7443)

        # Remove only the IPv6 meter; the port set and IPv4 rule remain.
        rules = run('nft', '-a', '-n', 'list', 'chain', 'inet', 'xui_telemt_meko', 'input')
        ipv6 = next(line for line in rules.splitlines() if 'meter tm6_7443 ' in line)
        handle = re.search(r'# handle (\d+)', ipv6).group(1)
        run('nft', 'delete', 'rule', 'inet', 'xui_telemt_meko', 'input', 'handle', handle)
        assert status()['installed'] == 'false', 'Missing IPv6 rule incorrectly verified'
        apply()
        healthy(7443)

        # Detached chains do not protect incoming traffic.
        run('nft', 'delete', 'table', 'inet', 'xui_telemt_meko')
        run('nft', 'add', 'table', 'inet', 'xui_telemt_meko')
        run('nft', 'add', 'chain', 'inet', 'xui_telemt_meko', 'input')
        assert status()['installed'] == 'false', 'Detached chain incorrectly verified'
        apply()
        healthy(7443)

        (configs / 'telemt-1.toml').unlink()
        apply()
        actual = status()
        assert actual['enabled'] == 'false' and actual['backend'] == 'none', actual
        assert actual['applied_ports'] == '' and actual['verified_ports'] == '', actual
        print('PASS: real nftables installation, update, drift repair and cleanup')


if __name__ == '__main__':
    main()
