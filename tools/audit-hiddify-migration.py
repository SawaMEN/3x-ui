#!/usr/bin/env python3
"""Check the stage-1 inventory offline; optionally verify a pinned upstream checkout."""

import argparse
import json
from pathlib import Path
import re
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]


def require(condition, message):
    if not condition:
        raise ValueError(message)


def git(path, *args):
    return subprocess.check_output(
        ["git", "-C", str(path), *args], text=True, stderr=subprocess.PIPE
    ).strip()


def audit(upstream):
    lock = json.loads((ROOT / "docs/hiddify-core-target.json").read_text())
    require(lock["schema_version"] == 1 and lock["stage"] == 1, "Unknown audit schema/stage")
    require(not lock["runtime_contract"]["activation_ready"], "Stage 1 cannot enable migration")
    protocols = []
    for path in (ROOT / "internal/database/model").glob("*.go"):
        if not path.name.endswith("_test.go"):
            protocols.extend(re.findall(r'\bProtocol\s*=\s*"([^"\n]+)"', path.read_text()))
    rows = lock["protocols"]
    names = [row["panel_protocol"] for row in rows]
    require(len(names) == len(set(names)), "Duplicate protocol in migration inventory")
    missing, stale = set(protocols) - set(names), set(names) - set(protocols)
    require(not missing and not stale, f"Inventory drift: missing={sorted(missing)}, stale={sorted(stale)}")
    report = (ROOT / "docs/hiddify-core-migration-stage-1.md").read_text()
    for row in rows:
        require(row["migration"] in {"native", "adapt", "external", "blocked"}, "Unknown migration status")
        require(bool(row["decision"].strip()), f"No migration decision for {row['panel_protocol']}")
        require(f"| `{row['panel_protocol']}` |" in report, f"No report row for {row['panel_protocol']}")
        require((row["target_kind"] is None) == (row["target_type"] is None), "Incomplete target identity")
    target = lock["target"]
    for rev in [target["commit"], *target["submodules"].values(), lock["panel_baseline"]]:
        require(bool(re.fullmatch(r"[0-9a-f]{40}", rev)), "Expected a full pinned commit")
    release = lock["verified_release"]
    require(bool(re.fullmatch(r"[0-9a-f]{64}", release["sha256"])), "Expected SHA-256")
    require(f"/download/{target['tag']}/{release['asset']}" in release["url"], "Release URL is not pinned")
    build = lock["panel_build"]
    require({"with_v2ray_api", "with_awg", "with_quic", "with_wireguard", "with_xui_panel"}.issubset(build["tags"]), "Panel build lacks required capabilities")
    require(build["toolchain"] == "go1.26.3", "Hiddify TLS dependencies require the pinned Go toolchain")
    require((ROOT / build["check_command_overlay"]).is_file(), "Missing check command overlay")
    require((ROOT / build["clash_user_overlay"]).is_file(), "Missing authenticated Clash user overlay")
    print(f"OK: {len(names)} protocol entries (including aliases), explicit decisions and release lock")
    if upstream is None:
        print("Upstream verification skipped; pass --upstream to verify source evidence")
        return
    upstream = upstream.resolve()
    require(git(upstream, "rev-parse", "HEAD") == target["commit"], "Upstream HEAD differs from lock")
    require(git(upstream, "rev-parse", f"{target['tag']}^{{commit}}") == target["commit"], "Upstream tag differs from lock")
    require(not git(upstream, "status", "--porcelain", "--untracked-files=no"), "Upstream has tracked modifications")
    for name, revision in target["submodules"].items():
        require(git(upstream / name, "rev-parse", "HEAD") == revision, f"Uninitialized or unpinned submodule: {name}")
        require(not git(upstream / name, "status", "--porcelain", "--untracked-files=no"), f"Modified submodule: {name}")
    for check in lock["source_checks"]:
        require(check["contains"] in (upstream / check["path"]).read_text(), f"Source evidence changed: {check['path']}")
    print("OK: upstream tag/commit, submodules, clean tracked source and evidence anchors")
    print("This audit does not establish traffic, quota, gateway or platform compatibility")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--upstream", type=Path, help="hiddify-core checkout with initialized pinned submodules")
    args = parser.parse_args()
    try:
        audit(args.upstream)
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
