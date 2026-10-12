#!/usr/bin/env python3
"""Track the latest source commits of the two owned kernel forks.

This updates the reproducible lock and panel identity, not installed binaries.
CI must build and test a candidate before committing these edits.
"""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import re
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "docs/hiddify-core-target.json"
RUNTIME = ROOT / "internal/hiddify/translate.go"
SHA = re.compile(r"^[0-9a-f]{40}$")
CORE_REPO = "https://github.com/SawaMEN/hiddify-core"
SING_REPO = "https://github.com/SawaMEN/hiddify-sing-box"


def git(*args, cwd=None):
    return subprocess.check_output(["git", *args], cwd=cwd, text=True).strip()


def latest(repo):
    lines = git("ls-remote", "--exit-code", repo, "refs/heads/main").splitlines()
    if len(lines) != 1:
        raise RuntimeError("Could not identify main HEAD: " + repo)
    revision, ref = lines[0].split()
    if ref != "refs/heads/main" or not SHA.fullmatch(revision):
        raise RuntimeError("Invalid HEAD: " + repo)
    return revision


def gitlink(path, revision, name):
    entry = git("ls-tree", revision, "--", name, cwd=path).split()
    if len(entry) != 4 or entry[:2] != ["160000", "commit"] or entry[3] != name:
        raise RuntimeError("Expected submodule gitlink: " + name)
    if not SHA.fullmatch(entry[2]):
        raise RuntimeError("Invalid submodule revision: " + name)
    return entry[2]


def update_runtime(runtime, old_version, old_revision, version, revision):
    for label, old, new in (("TargetVersion", old_version, version), ("TargetRevision", old_revision, revision)):
        pattern = re.compile(r'^([ \t]*' + label + r'[ \t]*=[ \t]*)"([^"\n]+)"([ \t]*)$', re.M)
        matches = list(pattern.finditer(runtime))
        if len(matches) != 1 or matches[0].group(2) != old:
            raise RuntimeError(label + " does not match current kernel lock")
        runtime = pattern.sub(lambda m: m.group(1) + '"' + new + '"' + m.group(3), runtime)
    return runtime


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write", action="store_true", help="update lock and matching Go constants")
    args = parser.parse_args()
    lock = json.loads(MANIFEST.read_text())
    target = lock["target"]
    if target["repository"] != CORE_REPO or target["submodule_repositories"]["hiddify-sing-box"] != SING_REPO:
        raise RuntimeError("Unexpected core or Sing-box repository")
    core = latest(CORE_REPO)
    sing = latest(SING_REPO)
    with tempfile.TemporaryDirectory(prefix="xui-kernel-sync-") as directory:
        git("init", "-q", directory)
        git("remote", "add", "origin", CORE_REPO, cwd=directory)
        git("fetch", "--depth=1", "origin", core, cwd=directory)
        ray = gitlink(directory, "FETCH_HEAD", "ray2sing")

    old_core, old_version = target["commit"], target["version"]
    old_sing, old_ray = target["submodules"]["hiddify-sing-box"], target["submodules"]["ray2sing"]
    if not all(SHA.fullmatch(v) for v in (old_core, old_sing, old_ray)):
        raise RuntimeError("Invalid existing kernel revision")
    suffix = re.search(r"(-panel[0-9]+)$", old_version)
    if suffix is None:
        raise RuntimeError("Unknown panel patch profile")
    version = f"sawamen-{core[:12]}-{sing[:12]}{suffix.group(1)}"
    runtime = update_runtime(RUNTIME.read_text(), old_version, old_core, version, core)
    if (core, sing, ray, version) == (old_core, old_sing, old_ray, old_version):
        print("Kernel already uses the latest revisions.")
        return

    target["commit"] = core
    target["version"] = version
    target["submodules"]["hiddify-sing-box"] = sing
    target["submodules"]["ray2sing"] = ray
    lock["panel_build"]["sing_box_version"] = "sawamen-" + sing[:12]
    lock["audit_date"] = datetime.now(timezone.utc).date().isoformat()
    print(f"Hiddify-core: {old_core} -> {core}")
    print(f"Sing-box:     {old_sing} -> {sing}")
    print(f"Ray2sing:     {old_ray} -> {ray}")
    print(f"Version:      {old_version} -> {version}")
    if args.write:
        MANIFEST.write_text(json.dumps(lock, ensure_ascii=False, indent=2) + "\n")
        RUNTIME.write_text(runtime)
    else:
        print("Dry run; --write required to make changes.")


if __name__ == "__main__":
    main()
