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

# Top-level fields drive the native outbound editor. Nested structures retain
# their JSON types and can be edited independently without replacing the draft.
ui = {}
for kind, name in sorted(registrations.items()):
    if kind not in output:
        continue
    ui[kind] = {key: value for key, value in fields(name).items() if '.' not in key}
    def ui_types(name):
        result = {}
        for key, go_type in structs.get(resolve(name), []):
            if not key:
                result.update(ui_types(go_type))
            elif go_type.lstrip('*') in ['byteformats.MemoryBytes', 'badoption.HTTPHeader', 'FwMark']:
                result[key] = {'byteformats.MemoryBytes': 'string', 'badoption.HTTPHeader': 'object', 'FwMark': 'number'}[go_type.lstrip('*')]
            elif go_type in ['[]string', 'badoption.Listable[string]']:
                result[key] = 'strings'
        return result
    ui[kind].update(ui_types(name))
ui_path = Path(__file__).resolve().parent.parent / 'frontend/src/pages/singbox/native-outbound-fields.json'
ui_path.write_text(json.dumps(ui, ensure_ascii=False, sort_keys=True, indent=2) + '\n')
