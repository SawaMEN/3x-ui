from pathlib import Path


def add_after(path: str, needle: str, addition: str) -> None:
    p = Path(path)
    text = p.read_text()
    if addition.strip() in text:
        return
    if needle not in text:
        raise RuntimeError(f"{path}: anchor not found: {needle!r}")
    p.write_text(text.replace(needle, needle + addition, 1))


add_after(
    "internal/web/service/xray.go",
    '\t"github.com/SawaMEN/3x-ui/v3/internal/logger"\n',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/tuic"\n',
)
add_after(
    "internal/web/service/inbound_clients_tuic_test.go",
    '\t"github.com/SawaMEN/3x-ui/v3/internal/database/model"\n',
    '\t"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"\n',
)
