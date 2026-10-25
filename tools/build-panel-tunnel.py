#!/usr/bin/env python3
"""Build the pinned protocol sidecar with mandatory panel payload routing."""
import argparse
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

REVISION = "56ce6f1aeea18aa69b9bc1d57557ed312b59e5f5"
ROOT = Path(__file__).resolve().parent.parent


def replace_once(path, old, new):
    text = path.read_text()
    if text.count(old) != 1:
        raise RuntimeError(f"Pinned source contract changed: {path.name}: {old!r}")
    path.write_text(text.replace(old, new, 1))


def prepare(source):
    if subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=source, text=True).strip() != REVISION:
        raise RuntimeError("Unexpected upstream revision")
    shutil.copyfile(ROOT / "deploy/panel-tunnel/egress.go.in", source / "internal/tunnel/panel_egress.go")
    replace_once(source / "internal/app/server.go", 'net.DialTimeout("tcp", destAddrStr, 10*time.Second)', 'tunnel.PanelDialTCP(destAddrStr)')
    replace_once(source / "internal/tunnel/mux_server.go", 'net.DialTimeout("tcp", addr, 10*time.Second)', 'PanelDialTCP(addr)')
    # Keep the upstream direct branch for upstream tests and client usage.
    # Panel starts the binary with -panel-egress, which always selects SOCKS.
    replace_once(source / "internal/tunnel/uot.go", 'func HandleUoTServer(conn net.Conn) error {', 'func HandleUoTServer(conn net.Conn) error {\n if *panelEgress != "" { return panelHandleUoT(conn) }')
    # The replaced direct dials were the only time references in these files.
    replace_once(source / "internal/tunnel/mux_server.go", '\n\t"time"', '')
    replace_once(source / "internal/app/server.go", '\n\t"time"', '')
    subprocess.run(["gofmt", "-w", "internal/tunnel/panel_egress.go", "internal/tunnel/mux_server.go", "internal/tunnel/uot.go", "internal/app/server.go"], cwd=source, check=True)


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--output", required=True)
    p.add_argument("--arch", default=os.environ.get("GOARCH", "amd64"), choices=["amd64", "arm64"])
    p.add_argument("--source", type=Path)
    p.add_argument("--test", action="store_true")
    args = p.parse_args()
    output = Path(args.output).resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="panel-tunnel-") as tmp:
        source = Path(tmp) / "source"
        if args.source:
            subprocess.run(["git", "clone", "--no-hardlinks", str(args.source), str(source)], check=True)
        else:
            subprocess.run(["git", "init", str(source)], check=True)
            subprocess.run(["git", "remote", "add", "origin", "https://github.com/SUDOKU-ASCII/sudoku.git"], cwd=source, check=True)
            subprocess.run(["git", "fetch", "--depth=1", "origin", REVISION], cwd=source, check=True)
        subprocess.run(["git", "checkout", "--detach", REVISION], cwd=source, check=True)
        prepare(source)
        if args.test:
            shutil.copyfile(ROOT / "deploy/panel-tunnel/egress_test.go.in", source / "internal/tunnel/panel_egress_test.go")
            test_env = dict(os.environ, CGO_ENABLED="0")
            # Release jobs can retain a cross C compiler from the panel build.
            # Run protocol tests on the build host, then cross-compile below.
            test_env.pop("GOOS", None)
            test_env.pop("GOARCH", None)
            subprocess.run(["go", "test", "./internal/tunnel", "./internal/app", "-count=1", "-timeout=120s"], cwd=source, env=test_env, check=True)
        env = dict(os.environ, GOOS="linux", GOARCH=args.arch, CGO_ENABLED="0")
        subprocess.run(["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(output), "./cmd/sudoku-tunnel"], cwd=source, env=env, check=True)
        # GPL source + exact extension accompany redistributable builds.
        shutil.copyfile(source / "LICENSE", output.parent / "panel-tunnel-LICENSE")
        subprocess.run(["tar", "czf", str(output.parent / "panel-tunnel-source.tar.gz"), "--exclude=.git", "-C", str(source), "."], check=True)

if __name__ == "__main__":
    main()
