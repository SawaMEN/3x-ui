#!/usr/bin/env python3
"""Reproducible per-commit inventory of the historic DEV→main window.

Never substitute the moving dev-latest tag for the pinned source commit.
Net changed files use git diff, not GitHub Compare's 300-entry limit.
Candidate test ownership is not a claim of manual behavioral verification.
"""
import argparse
import csv
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
DEV = "8b59a974c23ae4c7fb9072d46fa18abb5390761a"
HISTORIC_MAIN = "288becb7f31e004bb1f1e37d596aa0922ef38269"
MERGED_MAIN = "50a84c75409e6619de3a3abf14ab1e0eb42f17c8"

WINDOWS = (
    ("historic", DEV, HISTORIC_MAIN, 131, 57),
    ("later", HISTORIC_MAIN, MERGED_MAIN, 283, None),
    ("combined", DEV, MERGED_MAIN, 414, None),
)


def git(*args):
    return subprocess.check_output(("git", *args), cwd=ROOT)


def paths(*args):
    return sorted({item.decode("utf-8", "surrogateescape")
                   for item in git(*args).split(b"\0") if item})


def classify(path):
    if path.startswith(("frontend/", "internal/web/websocket/")):
        return "frontend-ws"
    if path.startswith(("internal/database/", "internal/recovery/")):
        return "database"
    if path.startswith(("internal/externalvpn/", "internal/singbox/", "internal/hiddify/",
                        "internal/tuic/", "internal/mtproto/")):
        return "protocol-core"
    if path.startswith(("internal/web/", "internal/sub/", "internal/clientlimit/")):
        return "backend-api"
    if path.startswith(("internal/logger/", "internal/util/")):
        return "runtime-utilities"
    if path.startswith(("docs/",)) or path.endswith(".md"):
        return "docs"
    if path.startswith((".github/", "deploy/", "tools/")) or path in (
        "install.sh", "update.sh", "core-migration.sh", "Dockerfile", "Makefile",
        "DockerInit.sh", "go.mod", "go.sum"):
        return "build-release"
    return "other-review"


CHECKS = {
    "frontend-ws": "frontend typecheck/Vitest/Chromium and websocket race",
    "database": "database migration and PostgreSQL parity",
    "protocol-core": "native protocol/sidecar and core config matrix",
    "backend-api": "service and auth regression with -race",
    "runtime-utilities": "runtime/logging/tail targeted tests",
    "docs": "docs CI and OpenAPI consistency",
    "build-release": "stage11 installer + native release",
    "other-review": "manual source/test assessment",
}


def inspect(name, base, head, expected_commits, expected_paths):
    if subprocess.run(["git", "merge-base", "--is-ancestor", base, head], cwd=ROOT).returncode:
        raise ValueError(f"{name}: pinned base is not an ancestor of head")
    sha_list = git("rev-list", "--reverse", "--topo-order", f"{base}..{head}").decode().split()
    net_paths = paths("diff", "--no-renames", "--name-only", "-z", base, head)
    if len(sha_list) != expected_commits:
        raise ValueError(f"{name}: expected {expected_commits} commits; got {len(sha_list)}")
    if expected_paths is not None and len(net_paths) != expected_paths:
        raise ValueError(f"{name}: expected {expected_paths} changed paths; got {len(net_paths)}")
    rows = []
    for sha in sha_list:
        parents = git("rev-list", "--parents", "-n", "1", sha).decode().split()[1:]
        files = paths("diff", "--no-renames", "--name-only", "-z", parents[0], sha)
        groups = sorted({classify(p) for p in files})
        rows.append({
            "sha": sha,
            "subject": git("show", "-s", "--format=%s", sha).decode("utf-8", "replace").strip(),
            "paths": files,
            "groups": groups,
            "candidate_tests": [CHECKS[g] for g in groups],
        })
    return {"base": base, "head": head, "commits": len(rows),
            "net_changed_paths": len(net_paths), "net_paths": net_paths,
            "per_commit": rows}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, default=Path("stage13-evidence"))
    args = parser.parse_args()
    output = args.output
    output.mkdir(parents=True, exist_ok=True)
    totals = {}
    for name, base, head, count, file_count in WINDOWS:
        result = inspect(name, base, head, count, file_count)
        (output / f"{name}.json").write_text(json.dumps(result, indent=2, ensure_ascii=False) + "\n")
        with (output / f"{name}.csv").open("w", newline="") as dest:
            writer = csv.writer(dest)
            writer.writerow(("commit", "subject", "changed_files", "areas", "candidate_tests"))
            for row in result["per_commit"]:
                writer.writerow((row["sha"], row["subject"], len(row["paths"]),
                                 "; ".join(row["groups"]), "; ".join(row["candidate_tests"])))
        totals[name] = {"commits": result["commits"], "net_changed_paths": result["net_changed_paths"]}
        print(name, totals[name])
    (output / "summary.json").write_text(json.dumps(totals, indent=2) + "\n")


if __name__ == "__main__":
    main()
