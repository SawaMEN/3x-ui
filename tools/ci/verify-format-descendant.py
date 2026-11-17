#!/usr/bin/env python3
"""Allow release reuse only when pinned oxfmt gives identical old/new sources."""
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)


def equivalent(base, head):
    formatter = ROOT / "frontend/node_modules/.bin/oxfmt"
    if not formatter.is_file():
        raise ValueError("Pinned frontend formatter is not installed")
    names = git("diff", "--name-only", "-z", base, head).split(b"\0")
    with tempfile.TemporaryDirectory(prefix="xui-format-check-") as directory:
        for index, raw in enumerate(filter(None, names)):
            name = raw.decode("utf-8")
            path = Path(name)
            if not name.startswith("frontend/src/") or path.suffix not in (".ts", ".tsx", ".js", ".jsx", ".css", ".json"):
                return False
            # Added/deleted files are application changes, even with bot metadata.
            sources = []
            for label, revision in (("base", base), ("head", head)):
                target = Path(directory) / label / str(index) / path.name
                target.parent.mkdir(parents=True)
                try:
                    target.write_bytes(git("show", revision + ":" + name))
                except subprocess.CalledProcessError:
                    return False
                subprocess.run([str(formatter), "--config", str(ROOT / "frontend/.oxfmtrc.json"), str(target)], cwd=ROOT, check=True,
                               stdout=subprocess.DEVNULL)
                sources.append(target.read_bytes())
            if sources[0] != sources[1]:
                return False
    return True


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit("usage: verify-format-descendant.py BASE HEAD")
    try:
        sys.exit(0 if equivalent(sys.argv[1], sys.argv[2]) else 1)
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print("Cannot verify formatter-only descendant: " + str(error), file=sys.stderr)
        sys.exit(1)
