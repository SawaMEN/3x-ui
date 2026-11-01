#!/usr/bin/env python3
"""Snapshot native URI option types from the owner kernel's Go source.
Usage: python3 tools/generate-native-import-options.py /path/to/hiddify-sing-box
Native JSON and options={} remain open-ended; this is only URL scalar decoding.
"""
import json
import re
import sys
from pathlib import Path

root = Path(sys.argv[1])
structs, aliases = {}, {}
def collect(source):
    for name, body in re.findall(r'type (\w+) struct\s*\{(?:[^\n]*\n)(.*?)\n\}', source, re.S):
        fields = []
        for line in body.splitlines():
            match = re.match(r'\s*\w+\s+(.+?)\s+`json:"([^",]+)', line)
            if match and match[2] != '-':
                fields.append((match[2], match[1].strip()))
            elif '`' not in line:
                embed = re.match(r'^\s*\*?(\w+)\s*(?://.*)?$', line)
                if embed:
                    fields.append(('', embed[1]))
        structs[name] = fields
    for name, target in re.findall(r'^type (\w+) (\w+)\s*(?://[^\n]*)?$', source, re.M):
        aliases[name] = target

for file in (root / 'option').glob('*.go'):
    collect(file.read_text())
patch_sources = []
for patch in (Path(__file__).resolve().parent.parent/'deploy/hiddify').glob('*.patch'):
    added = '\n'.join(line[1:] for line in patch.read_text().splitlines() if line.startswith('+') and not line.startswith('+++'))
    collect(added)
    patch_sources.append(added)

constants = dict(re.findall(r'(Type\w+)\s*=\s*"([^"]+)"', (root/'constant/proxy.go').read_text()))
registrations = {}
for file in (root/'protocol').rglob('*.go'):
    if file.name.endswith('_test.go'):
        continue
    for name, constant in re.findall(r'(?:outbound|endpoint)\.Register\[option\.(\w+)\]\(registry, C\.(Type\w+)', file.read_text()):
        if constant in constants:
            registrations[constants[constant]] = name
for source in patch_sources:
    for name, kind in re.findall(r'outbound\.Register\[option\.(\w+)\]\(registry, "([^\"]+)"', source):
        registrations[kind] = name
# Some protocol registries use convenience wrappers and live in include.
registrations.update({'sudoku':'SudokuOutboundOptions','openflux':'OpenFluxOutboundOptions','fptn':'FPTNOutboundOptions','pingtunnel':'PingTunnelOutboundOptions','balancer':'BalancerOutboundOptions','mieru':'MieruOutboundOptions','awg':'AwgEndpointOptions'})

def resolve(name):
    for _ in range(10):
        if name not in aliases:
            break
        name = aliases[name]
    return name

def describe(kind):
    kind = kind.lstrip('*')
    if kind == 'string':
        return 'string'
    if kind == 'bool':
        return 'boolean'
    if re.fullmatch(r'(u?int\d*|float\d+)', kind):
        return 'number'
    if kind.startswith('[]') or 'Listable[' in kind or kind == 'NetworkList':
        return 'array'
    if kind.startswith('map[') or kind == 'json.RawMessage' or resolve(kind) in structs:
        return 'object'
    if any(word in kind for word in ('Duration','Addr','Prefix','DomainStrategy','NetworkStrategy')):
        return 'string'
    return 'json'

def fields(name, prefix='', depth=0):
    if depth > 8:
        return {}
    result = {}
    for key, kind in structs.get(resolve(name), []):
        if not key:
            result.update(fields(kind,prefix,depth+1))
            continue
        path = prefix+key
        result[path] = describe(kind)
        target = resolve(kind.lstrip('*'))
        if target in structs:
            result.update(fields(target,path+'.',depth+1))
    return result

output = {kind:fields(name) for kind,name in sorted(registrations.items()) if kind not in ['hinvalid','dns','shadowsocksr']}
# V2Ray's transport union marshals its selected options into transport directly.
transport_fields = {}
for name in ['V2RayHTTPOptions','V2RayWebsocketOptions','V2RayGRPCOptions','V2RayHTTPUpgradeOptions','V2RayXHTTPOptions','V2RayKCPOptions']:
    transport_fields.update(fields(name,'transport.'))
for kind in ['vless','vmess','trojan']:
    output[kind].update(transport_fields)

for kind, name in {'http':'V2RayHTTPOptions','ws':'V2RayWebsocketOptions','grpc':'V2RayGRPCOptions','httpupgrade':'V2RayHTTPUpgradeOptions','xhttp':'V2RayXHTTPOptions','kcp':'V2RayKCPOptions'}.items():
    output['__transport:'+kind] = fields(name)
path=Path(__file__).resolve().parent.parent/'internal/web/service/native_import_options.json'
path.write_text(json.dumps(output,ensure_ascii=False,sort_keys=True,indent=2)+'\n')
print(f'Generated native URI options for {len(registrations)} types and transport variants')

# Rich UI descriptors preserve nested structure, numeric ranges and per-field
# enums. Definitions are shared instead of duplicating TLS/dialer schemas for
# every protocol. Unknown external/custom unions remain editable as JSON.
ui_structs = {}

def parse_ui_body(body, owner):
    lines = body.splitlines()
    result = []
    index = 0
    while index < len(lines):
        line = lines[index].strip()
        index += 1
        if not line or line.startswith('//'):
            continue
        inline = re.match(r'(\w+)\s+(\[\])?struct\s*\{', line)
        if inline:
            start = index
            level = 1
            while index < len(lines):
                level += lines[index].count('{') - lines[index].count('}')
                index += 1
                if level == 0:
                    break
            child = owner + '_' + inline[1]
            ui_structs[child] = parse_ui_body('\n'.join(lines[start:index-1]), child)
            line = inline[1] + ' ' + ('[]' if inline[2] else '') + child + ' ' + lines[index-1].strip()[1:]
        match = re.match(r'(\w+)\s+(.+?)\s+`([^`]+)`', line)
        if match:
            tag = re.search(r'json:"([^",]+)([^\"]*)"', match[3])
            if tag and tag[1] != '-' and not match[1].startswith('Deprecated'):
                enum = re.search(r'enum:"([^\"]+)"', match[3])
                result.append((tag[1], match[2].strip(), enum[1].split(',') if enum else None, 'omitempty' not in tag[2]))
        else:
            embed = re.fullmatch(r'\*?([\w.]+)(?:\s*//.*)?', line)
            if embed:
                result.append(('', embed[1], None, False))
    return result

def collect_ui(source):
    for match in re.finditer(r'type (\w+) struct\s*\{', source):
        start, level, end = match.end(), 1, match.end()
        # Struct declarations have no string values; ignore comments while
        # counting braces so example snippets cannot affect nesting.
        for line in source[start:].splitlines(keepends=True):
            clean = line.split('//', 1)[0]
            level += clean.count('{') - clean.count('}')
            end += len(line)
            if level == 0:
                break
        body = source[start:end].rsplit('}', 1)[0]
        ui_structs[match[1]] = parse_ui_body(body, match[1])

for file in sorted((root / 'option').glob('*.go')):
    collect_ui(file.read_text())
collect_ui((root / 'constant/warp.go').read_text())
for source in patch_sources:
    collect_ui(source)

ui_definitions = {}

def ui_resolve(name):
    return resolve(name.removeprefix('C.'))

def ui_descriptor(go_type, key='', enum=None, depth=0):
    kind = go_type.lstrip('*')
    scalar = {'string': 'string', 'bool': 'boolean', 'byteformats.MemoryBytes': 'string',
              'FwMark': 'number', 'NetworkList': 'network', 'badoption.HTTPHeader': 'headers'}
    result = {'kind': scalar.get(kind, 'json')}
    if re.fullmatch(r'(u?int\d*|float\d+)', kind):
        result = {'kind': 'number', 'integer': not kind.startswith('float')}
        if kind.startswith('uint'):
            result['min'] = 0
        if kind in ['uint8', 'uint16']:
            result['max'] = 2 ** int(kind[4:]) - 1
    elif kind.startswith('[]') or kind.startswith('badoption.Listable['):
        item = kind[2:] if kind.startswith('[]') else kind[len('badoption.Listable['):-1]
        result = {'kind': 'array', 'item': ui_descriptor(item, depth=depth+1)}
        if result['item']['kind'] == 'string':
            result = {'kind': 'strings'}
    elif kind in ['json.RawMessage', 'any'] or kind.startswith('map['):
        result = {'kind': 'json'}
    elif ui_resolve(kind) in ui_structs:
        target = ui_resolve(kind)
        result = {'kind': 'object', 'ref': target}
        if target not in ui_definitions and depth < 12:
            ui_definitions[target] = {}  # break recursive definitions
            ui_definitions[target] = ui_fields(target, depth+1)
        if target in ['UDPOverTCPOptions', '_UDPOverTCPOptions']:
            result['boolean'] = True
        if target in ['DomainResolveOptions', '_DomainResolveOptions', 'HTTPClientOptions', 'HTTPClient', '_HTTPClientOptions']:
            result['string'] = True
    elif any(word in kind for word in ('Duration', 'Addr', 'Prefix', 'DomainStrategy', 'NetworkStrategy', 'InterfaceType', 'UDPNATBehavior')):
        result = {'kind': 'string'}
    if enum:
        result['enum'] = [int(v) for v in enum] if result['kind'] == 'number' else enum
    if key in ['insecure_concurrency', 'min_idle_session', 'up_mbps', 'down_mbps', 'max_concurrent_streams']:
        result['min'] = 0
    return result

def ui_fields(name, depth=0):
    result = {}
    for key, kind, enum, required in ui_structs.get(ui_resolve(name), []):
        if key:
            result[key] = ui_descriptor(kind, key, enum, depth)
            if required:
                result[key]['required'] = True
        elif depth < 12:
            result.update(ui_fields(kind, depth+1))
    return result

ui = {kind: ui_fields(name) for kind, name in sorted(registrations.items()) if kind in output}
# These structs marshal version-specific options into the surrounding object.
for name, versions in [('SnellObfsClientOptions', [4]), ('SnellV6Options', [6])]:
    ui['snell'].update({key: {**value, 'when': {'version': versions}} for key, value in ui_fields(name).items()})
for key, value in ui_fields('QUICOptions').items():
    ui['http'][key] = {**value, 'when': {'version': [3] if key in ['initial_packet_size', 'disable_path_mtu_discovery'] else [0, 2, 3]}}
ui_definitions['_Hysteria2Obfs'] = {**ui_definitions.get('_Hysteria2Obfs', {}), **{
    key: {**value, 'when': {'type': ['gecko']}} for key, value in ui_fields('Hysteria2ObfsGecko').items()
}}
# Options parsed by helpers rather than Go enum tags.
ui['mieru']['multiplexing']['enum'] = ['MULTIPLEXING_DEFAULT', 'MULTIPLEXING_LOW', 'MULTIPLEXING_MIDDLE', 'MULTIPLEXING_HIGH']
ui['mieru']['handshake_mode']['enum'] = ['HANDSHAKE_DEFAULT', 'HANDSHAKE_STANDARD', 'HANDSHAKE_NO_WAIT']
ui_definitions['MieruPortBinding']['protocol']['enum'] = ['TCP', 'UDP']
ui_definitions['OpenFluxTransportOptions']['type']['enum'] = ['direct', 'yandex', 'vyandex', 'boards', 'mailru']
ui['openflux']['codec']['enum'] = ['batched']
ui['balancer']['strategy']['enum'] = ['round-robin', 'consistent-hashing', 'sticky-sessions', 'lowest-delay']
# Raw native wrapper unions are resolved by the kernel, not by a fixed
# single-protocol form. Preserve their complete JSON instead of dropping Options.
for schema in [*ui.values(), *ui_definitions.values()]:
    for key, descriptor in schema.items():
        if descriptor.get('ref') in ['_Outbound', '_Inbound', 'Outbound', 'Inbound']:
            schema[key] = {'kind': 'json', 'required': descriptor.get('required', False)}

ui_path = Path(__file__).resolve().parent.parent / 'frontend/src/pages/singbox/native-outbound-fields.json'
ui_path.write_text(json.dumps({'protocols': ui, 'definitions': ui_definitions,
    'dialerFields': list(ui_fields('DialerOptions'))}, ensure_ascii=False, sort_keys=True, indent=2) + '\n')
