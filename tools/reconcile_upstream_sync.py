from pathlib import Path
import subprocess

ROOT = Path('.')


def read(path: str) -> str:
    return (ROOT / path).read_text()


def write(path: str, text: str) -> None:
    p = ROOT / path
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(text)


def add_after(path: str, needle: str, addition: str) -> None:
    text = read(path)
    if addition.strip() in text:
        return
    if needle not in text:
        raise RuntimeError(f'{path}: insertion anchor not found: {needle!r}')
    write(path, text.replace(needle, needle + addition, 1))


# Drop the temporary namespace shim; restore real imports instead.
compat = ROOT / 'internal/web/service/upstream_tuic_compat.go'
if compat.exists():
    compat.unlink()

add_after(
    'internal/web/service/inbound_amneziawg.go',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/logger"\n',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/tuic"\n',
)
add_after(
    'internal/web/service/port_conflict.go',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/mieru"\n',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/tuic"\n',
)

# Build the local email list needed by the upstream TUIC traffic-id lookup.
path = 'internal/web/service/inbound.go'
text = read(path)
start = text.find('func (s *InboundService) buildInboundForLocalRuntime')
marker = '\ttrafficIDs := make(map[string]int)\n\tif inbound.Protocol == model.TUIC {'
if start >= 0 and marker in text[start:] and '\temails := make([]string, 0, len(clients))\n' not in text[start:]:
    replacement = '''\temails := make([]string, 0, len(clients))
\tfor _, client := range clients {
\t\tif c, ok := client.(map[string]any); ok {
\t\t\temail, _ := c["email"].(string)
\t\t\tif email != "" {
\t\t\t\temails = append(emails, email)
\t\t\t}
\t\t}
\t}

\ttrafficIDs := make(map[string]int)
\tif inbound.Protocol == model.TUIC {'''
    text = text[:start] + text[start:].replace(marker, replacement, 1)
    write(path, text)

add_after(
    'internal/web/service/server.go',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/util/common"\n',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"\n',
)
add_after(
    'internal/web/service/panel/panel.go',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/logger"\n',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/util/version"\n',
)
add_after(
    'internal/web/service/tgbot/tgbot.go',
    '\t"math/big"\n',
    '\t"net"\n',
)
add_after(
    'internal/web/controller/server.go',
    '\t"encoding/json"\n',
    '\t"errors"\n',
)
add_after(
    'internal/web/controller/server.go',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/logger"\n',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"\n',
)

path = 'internal/web/web.go'
text = read(path)
fn = 'func (s *Server) stop(stopXray bool, stopTgBot bool) error {\n'
if fn in text and fn + '\tvar err1, err2 error\n' not in text:
    write(path, text.replace(fn, fn + '\tvar err1, err2 error\n', 1))

write(
    'internal/web/service/sync_port_helpers.go',
    '''package service

import "github.com/SawaMEN/3x-ui/v3/internal/database/model"

var loopbackBind = "127.0.0.1"

func inboundBindAddr(ib *model.Inbound) string {
\tif ib == nil {
\t\treturn ""
\t}
\treturn ib.Listen
}
''',
)

# Keep native TUIC accounting coherent: job and journal come from one upstream version.
for rel in [
    'internal/web/job/tuic_job.go',
    'internal/web/job/tuic_journal.go',
    'internal/web/job/tuic_job_test.go',
    'internal/web/job/tuic_journal_test.go',
]:
    data = subprocess.check_output(['git', 'show', f'upstream/main:{rel}'], text=True)
    data = data.replace('github.com/mhsanaei/3x-ui/v3', 'github.com/SawaMEN/3x-ui/v3')
    write(rel, data)

files = [
    'internal/web/service/inbound.go',
    'internal/web/service/inbound_amneziawg.go',
    'internal/web/service/port_conflict.go',
    'internal/web/service/server.go',
    'internal/web/service/panel/panel.go',
    'internal/web/service/tgbot/tgbot.go',
    'internal/web/controller/server.go',
    'internal/web/web.go',
    'internal/web/service/sync_port_helpers.go',
    'internal/web/job/tuic_job.go',
    'internal/web/job/tuic_journal.go',
    'internal/web/job/tuic_job_test.go',
    'internal/web/job/tuic_journal_test.go',
]
subprocess.run(['gofmt', '-w', *files], check=True)

out = Path('/tmp/reconciled')
for rel in files:
    target = out / rel
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes((ROOT / rel).read_bytes())
(out / 'DELETE_FILES.txt').write_text('internal/web/service/upstream_tuic_compat.go\n')
