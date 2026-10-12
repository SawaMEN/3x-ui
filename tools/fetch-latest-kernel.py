#!/usr/bin/env python3
"""Download the latest complete published SawaMEN/hiddify-core release.

Includes prereleases (GitHub /releases/latest omits them). A shared tag
is resolved once per panel workflow, so amd64 and arm64 use the same build.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import struct
import subprocess
import sys
import tarfile
import tempfile
import time
from urllib.parse import quote
from urllib.request import Request, urlopen

API = "https://api.github.com/repos/SawaMEN/hiddify-core"
PREFIX = "https://github.com/SawaMEN/hiddify-core/releases/download/"
TAGS = {
    "with_xui_panel", "with_v2ray_api", "with_awg", "with_quic",
    "with_wireguard", "with_clash_api", "with_utls", "with_masque",
    "with_gvisor", "with_naive_outbound", "tfogo_checklinkname0",
}


def fetch(url, destination=None):
    headers = {"User-Agent": "3x-ui-core-release-fetch"}
    if url.startswith(API) and (os.getenv("GH_TOKEN") or os.getenv("GITHUB_TOKEN")):
        headers["Authorization"] = "Bearer " + (os.getenv("GH_TOKEN") or os.getenv("GITHUB_TOKEN"))
    for attempt in range(4):
        try:
            with urlopen(Request(url, headers=headers), timeout=90) as response:
                if destination is None:
                    return response.read()
                with open(destination, "wb") as output:
                    shutil.copyfileobj(response, output, 1024 * 1024)
                return
        except Exception:
            if attempt == 3:
                raise
            time.sleep(2 * (attempt + 1))


def json_api(path):
    return json.loads(fetch(API + path))


def complete(release):
    required = {
        f"sing-box-linux-{arch}.tar.gz{suffix}"
        for arch in ("amd64", "arm64") for suffix in ("", ".sha256")
    }
    return (not release.get("draft") and bool(release.get("published_at"))
            and required <= {asset["name"] for asset in release.get("assets", [])})


def resolve(tag=None):
    if tag:
        if not re.fullmatch(r"[A-Za-z0-9._-]+", tag):
            raise ValueError("Invalid release tag")
        release = json_api("/releases/tags/" + quote(tag, safe=""))
        if not complete(release):
            raise ValueError("Release has no complete binaries and SHA-256 for both architectures")
        return release
    releases = [release for release in json_api("/releases?per_page=100") if complete(release)]
    if not releases:
        raise RuntimeError("No complete published Sing-box release")
    return max(releases, key=lambda release: (release["published_at"], release["id"]))


def checksum(file, checksum_file):
    words = Path(checksum_file).read_text().strip().split()
    if len(words) != 2 or not re.fullmatch(r"[0-9a-fA-F]{64}", words[0]):
        raise ValueError("Invalid SHA-256 checksum format: " + Path(checksum_file).name)
    if words[1].lstrip("*") != Path(file).name:
        raise ValueError("SHA-256 sidecar references wrong file")
    digest = hashlib.sha256()
    with open(file, "rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    if digest.hexdigest() != words[0].lower():
        raise ValueError("SHA-256 mismatch: " + Path(file).name)


def install(release, arch, destination):
    tag = release["tag_name"]
    name = f"sing-box-linux-{arch}.tar.gz"
    assets = {a["name"]: a for a in release["assets"]}
    destination.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".core-fetch-", dir=destination) as directory:
        temp = Path(directory)
        for asset_name in (name, name + ".sha256"):
            url = assets[asset_name]["browser_download_url"]
            if not url.startswith(PREFIX + tag + "/"):
                raise ValueError("Unexpected kernel asset URL")
            fetch(url, temp / asset_name)
        checksum(temp / name, temp / (name + ".sha256"))
        binary = f"hiddify-core-linux-{arch}"
        selected = {binary, binary + ".sha256", "libcronet.so", "libcronet.so.sha256", "target.json"}
        with tarfile.open(temp / name, "r:gz") as package:
            entries = package.getmembers()
            if {m.name for m in entries} != selected or any(not m.isfile() or m.size > 300 * 1024 * 1024 for m in entries):
                raise ValueError("Unexpected or unsafe files in kernel release archive")
            for entry in entries:
                source = package.extractfile(entry)
                if source is None:
                    raise ValueError("Missing kernel archive member")
                with source, open(temp / entry.name, "wb") as output:
                    shutil.copyfileobj(source, output, 1024 * 1024)
        checksum(temp / binary, temp / (binary + ".sha256"))
        checksum(temp / "libcronet.so", temp / "libcronet.so.sha256")
        manifest = json.loads((temp / "target.json").read_text())
        profile = manifest["panel_build"]
        source = manifest["target"]
        if source["repository"] != "https://github.com/SawaMEN/hiddify-core" or source["version"] != tag:
            raise ValueError("Kernel version/source differs from release")
        if not TAGS.issubset(profile["tags"]) or not ({"with_purego", "with_musl"} & set(profile["tags"])):
            raise ValueError("Kernel build is missing mandatory panel features")
        header = (temp / binary).read_bytes()[:20]
        if len(header) != 20 or header[:6] != b"\x7fELF\x02\x01" or struct.unpack_from("<H", header, 18)[0] != {"amd64": 62, "arm64": 183}[arch]:
            raise ValueError("Downloaded kernel is not a matching 64-bit ELF executable")
        (temp / binary).chmod(0o755)
        if sys.platform == "linux" and {"x86_64": "amd64", "aarch64": "arm64"}.get(platform.machine()) == arch:
            version = subprocess.check_output([str(temp / binary), "version"], text=True, cwd=temp, timeout=30)
            if (not version.startswith("hiddify-core version " + tag + " ")
                    or "Revision: " + source["commit"] not in version):
                raise ValueError("Kernel executable does not match manifest identity")
        for file in selected:
            os.replace(temp / file, destination / file)
        print(f"Included verified core {tag} ({arch})", file=sys.stderr)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--latest-tag", action="store_true")
    parser.add_argument("--arch", choices=("amd64", "arm64"))
    parser.add_argument("--tag")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    release = resolve(args.tag)
    if args.latest_tag:
        print(release["tag_name"])
    else:
        if not args.arch or args.output is None:
            parser.error("--arch and --output are required")
        install(release, args.arch, args.output)


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(f"Failed to fetch verified kernel: {error}", file=sys.stderr)
        sys.exit(1)
