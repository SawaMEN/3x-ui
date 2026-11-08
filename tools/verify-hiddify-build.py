#!/usr/bin/env python3
"""Validate pinned runtime kernel sources, dependencies, and build overlays."""

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


def verify_fork(fork):
    lock = json.loads((ROOT / "deploy/hiddify/fork-contract.json").read_text())
    require(lock["schema_version"] == 1 and lock["stage"] == 1, "Unknown fork schema/stage")
    require(not lock["activation_ready"] and not lock["automatic_migration_ready"], "Unverified fork cannot enable migration")
    target = lock["target"]
    require(target["repository"] == "https://github.com/SawaMEN/hiddify-core", "Fork must be owner-managed")
    for revision in [lock["panel_baseline"], target["commit"], *target["submodules"].values()]:
        require(bool(re.fullmatch(r"[0-9a-f]{40}", revision)), "Expected pinned fork revision")
    rows = lock["protocols"]
    names = [row["panel_protocol"] for row in rows]
    require(len(names) == len(set(names)), "Duplicate fork protocol")
    for row in rows:
        require(row["migration"] in {"native", "adapt", "external", "blocked"}, "Unknown fork decision")
        require(bool(row["decision"].strip()), "Missing fork decision")
        require((row["target_kind"] is None) == (row["target_type"] is None), "Incomplete fork target identity")
    stats = lock["stats_contract"]
    client = (ROOT / "internal/singbox/v2rayapi.go").read_text()
    require(stats["panel_method"] in client, "Panel statistics contract changed; recheck fork compatibility")
    require(stats["compatible"] == (stats["method"] == stats["panel_method"]), "Incorrect statistics compatibility")
    print(f"OK: owner fork inventory ({len(names)} entries), pinned revisions, explicit API mismatch")
    if fork is None:
        print("Fork source verification skipped; pass --fork to check the pinned owner checkout")
        return
    fork = fork.resolve()
    require(git(fork, "remote", "get-url", "origin").removesuffix(".git") == target["repository"], "Wrong fork origin")
    require(git(fork, "rev-parse", "HEAD") == target["commit"], "Fork HEAD differs from lock")
    require(not git(fork, "status", "--porcelain", "--untracked-files=no"), "Fork has tracked changes")
    for name, revision in target["submodules"].items():
        require(git(fork, "ls-tree", "HEAD", name).split()[2] == revision, f"Wrong fork gitlink: {name}")
        require(git(fork / name, "rev-parse", "HEAD") == revision, f"Unpinned fork submodule: {name}")
        require(git(fork / name, "remote", "get-url", "origin").removesuffix(".git") == target["submodule_repositories"][name], f"Wrong fork submodule origin: {name}")
        require(not git(fork / name, "status", "--porcelain", "--untracked-files=no"), f"Modified fork submodule: {name}")
    for check in lock["source_checks"]:
        require(check["contains"] in (fork / check["path"]).read_text(), f"Fork source evidence changed: {check['path']}")
    print("OK: owner fork HEAD/gitlinks/origins and source anchors; no runtime/traffic validation")


def validate_runtime_security_dependencies(pins, builder):
    """Validate minimum patched versions and the build's single source of truth.

    The manifest owns the exact pins. Duplicating their entire version map in
    duplicating these versions made each security update fail CI even when the builder read
    the new manifest correctly.
    """
    floors = {
        "golang.org/x/net": (0, 62, 0),
        "golang.org/x/crypto": (0, 58, 1),
        "github.com/go-chi/chi/v5": (5, 3, 0),
        "github.com/google/cel-go": (0, 30, 0),
        "github.com/andybalholm/brotli": (1, 2, 7),
        "github.com/klauspost/compress": (1, 20, 2),
    }
    require(isinstance(pins, dict) and set(pins) == set(floors),
            "Runtime security dependency set differs from required modules")
    for module, minimum in floors.items():
        version = pins[module]
        match = re.fullmatch(r"v(\d+)\.(\d+)\.(\d+)(?:[-+].*)?", version) if isinstance(version, str) else None
        require(match is not None and tuple(map(int, match.groups())) >= minimum,
                f"Unpatched or invalid runtime security pin: {module}={version}")
    # The builder must consume the *entire* manifest mapping; an independently
    # copied version list would allow the audit and actual binary to diverge.
    require("['panel_build']['security_dependencies']" in builder
            and 'go mod edit -go=' in builder
            and '"${security_flags[@]}"' in builder
            and 'go mod download "${security_dependencies[@]}"' in builder,
            "Runtime build script does not enforce the pinned dependency set")


def verify_build(source):
    lock = json.loads((ROOT / "deploy/hiddify/hiddify-core-target.json").read_text())
    require(lock["schema_version"] == 2 and lock["stage"] in (2, 4), "Unknown runtime lock schema/stage")
    target, build = lock["target"], lock["panel_build"]
    fork = json.loads((ROOT / "deploy/hiddify/fork-contract.json").read_text())
    require(target["repository"] == fork["target"]["repository"], "Runtime must use the pinned owner repository")
    # The fork audit is historical; the actively built kernel lock advances independently.
    require(not lock["runtime_contract"]["automatic_migration_ready"], "Runtime profile cannot enable automatic migration before integration gates")
    require(build["toolchain"] == "go1.26.10", "Runtime must use the pinned security-patched toolchain")
    require(tuple(map(int, build["toolchain"].removeprefix("go").split("."))) >= tuple(map(int, fork["required_go_directive"].split("."))), "Runtime toolchain is older than the pinned source requirement")
    validate_runtime_security_dependencies(
        build["security_dependencies"], (ROOT / "tools/build-hiddify-core.sh").read_text()
    )
    require({"with_xui_panel", "with_v2ray_api", "with_awg", "with_quic", "with_wireguard", "with_masque", "with_clash_api"}.issubset(build["tags"]), "Runtime build lacks required tags")
    process = (ROOT / "internal/hiddify/process.go").read_text()
    tag_block = re.search(r"var requiredBuildTags = \[\]string\{([^}]+)\}", process)
    require(tag_block is not None, "Runtime required build tags missing")
    runtime_tags = re.findall(r'"([^"\n]+)"', tag_block.group(1))
    require(set(runtime_tags) == set(build["tags"]) - {"with_purego"}, "Runtime required tags differ from build lock")
    profile = (ROOT / "internal/hiddify/translate.go").read_text()
    require(target["version"] in profile and target["commit"] in profile, "Runtime process identity differs from build lock")
    require(lock["runtime_contract"]["stats_grpc_method"] == fork["stats_contract"]["method"], "Wrong runtime statistics method")
    require(lock["runtime_contract"]["stats_grpc_method"] in profile, "Runtime statistics client differs from lock")
    require(build["protection_dependency_versions"] == {"github.com/metacubex/utls": "v1.8.8", "github.com/starifly/sing-vmess": "v0.2.7-mod.9", "github.com/xtls/xray-core": "v1.260327.1-0.20260728075948-5ca6f4b7d4dc"}, "Protection dependency versions differ from the pinned build script")
    for key in ("check_command_overlay", "clash_user_overlay", "sniff_override_overlay", "sniff_override_test_overlay", "http1_options_overlay", "http1_options_test_overlay", "protection_options_overlay", "utls_protection_overlay", "vision_protection_overlay", "reality_protection_test_overlay", "vision_preconnect_test_overlay", "vision_padding_test_overlay", "transport_balancer_overlay", "xray_runtime_compat_overlay", "kcp_transport_test_overlay", "xhttp_extension_test_overlay", "balancer_strategy_test_overlay", "reverse_overlay", "reverse_protocol_overlay", "reverse_runtime_test_overlay", "reverse_vision_test_overlay", "runtime_dependencies_overlay", "vless_handshake_overlay", "vless_handshake_test_overlay", "explicit_cipher_overlay", "explicit_cipher_test_overlay", "stream_handshake_overlay", "stream_upgrade_test_overlay", "stream_trojan_test_overlay"):
        require((ROOT / build[key]).is_file(), "Missing runtime overlay: " + key)
    print(f"OK: stage-{lock['stage']} build/runtime identity, owner revisions, toolchain, tags and statistics profile")
    if source is None:
        return
    source = source.resolve()
    require(git(source, "remote", "get-url", "origin").removesuffix(".git") == target["repository"], "Wrong runtime source origin")
    require(git(source, "rev-parse", "HEAD") == target["commit"], "Runtime source HEAD differs from lock")
    require(not git(source, "status", "--porcelain", "--ignore-submodules=all", "--untracked-files=no"), "Runtime source has tracked changes")
    for name, revision in target["submodules"].items():
        if name != "hiddify-sing-box":
            require(git(source, "ls-tree", "HEAD", name).split()[2] == revision, "Runtime gitlink differs: " + name)
        require(git(source / name, "rev-parse", "HEAD") == revision, "Runtime submodule unpinned: " + name)
        require(git(source / name, "remote", "get-url", "origin").removesuffix(".git") == target["submodule_repositories"][name], "Wrong runtime submodule origin: " + name)
        require(not git(source / name, "status", "--porcelain", "--untracked-files=no"), "Runtime submodule has changes: " + name)
    for status in git(source, "submodule", "status", "--recursive").splitlines():
        if status.startswith("+") and re.search(r"\shiddify-sing-box(?:\s|\()", status):
            continue  # Intentional independent fork revision, checked above.
        require(not status.startswith(("-", "+", "U")), "Uninitialized or modified nested runtime submodule")
    for check in lock["source_checks"]:
        require(check["contains"] in (source / check["path"]).read_text(), "Runtime source evidence changed: " + check["path"])
    print("OK: runtime source/gitlinks/origins and source anchors before overlays")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--upstream", type=Path, help="active owner checkout with initialized pinned submodules")
    parser.add_argument("--fork", type=Path, help="optional owner fork checkout with pinned submodules")
    args = parser.parse_args()
    try:
        verify_build(args.upstream)
        verify_fork(args.fork)
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
