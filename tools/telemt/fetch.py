#!/usr/bin/env python3
"""Fetch unmodified official Telemt Linux musl releases with upstream checksums."""
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

API = "https://api.github.com/repos/telemt/telemt"
PREFIX = "https://github.com/telemt/telemt/releases/download/"
ASSETS = {"amd64": "telemt-x86_64-linux-musl.tar.gz", "arm64": "telemt-aarch64-linux-musl.tar.gz"}

def fetch(url, path=None):
    headers = {"User-Agent": "3x-ui-official-telemt"}
    token = os.getenv("GH_TOKEN") or os.getenv("GITHUB_TOKEN")
    if url.startswith(API) and token:
        headers["Authorization"] = "Bearer " + token
    for attempt in range(4):
        try:
            with urlopen(Request(url, headers=headers), timeout=90) as response:
                if path is None:
                    return response.read()
                with open(path, "wb") as output:
                    shutil.copyfileobj(response, output, 1 << 20)
                return
        except Exception:
            if attempt == 3:
                raise
            time.sleep(attempt + 1)

def resolve(tag=None):
    if tag is not None and not re.fullmatch(r"v?[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise ValueError("Only numeric official release tags are accepted")
    uri = "/releases/tags/" + quote(tag, safe="") if tag else "/releases/latest"
    release = json.loads(fetch(API + uri))
    version = release.get("tag_name", "")
    if not re.fullmatch(r"v?[0-9]+\.[0-9]+\.[0-9]+", version) or (tag is not None and tag != version):
        raise ValueError("Invalid official Telemt release version")
    if release.get("draft") or release.get("prerelease") or not release.get("published_at"):
        raise ValueError("Telemt release is not stable")
    assets = {a["name"] for a in release.get("assets", [])}
    required = {a for a in ASSETS.values()} | {a + ".sha256" for a in ASSETS.values()}
    if not required.issubset(assets):
        raise ValueError("Official Telemt release lacks an architecture or SHA-256 sidecar")
    return release

def verify(archive, sidecar, digest=None):
    fields = sidecar.read_text().strip().split()
    if len(fields) != 2 or not re.fullmatch(r"[0-9a-fA-F]{64}", fields[0]) or fields[1].lstrip("*") != archive.name:
        raise ValueError("Invalid official SHA-256 sidecar")
    actual = hashlib.sha256(archive.read_bytes()).hexdigest()
    if actual != fields[0].lower():
        raise ValueError("Official Telemt SHA-256 mismatch")
    if digest and digest != "sha256:" + actual:
        raise ValueError("GitHub asset digest differs from official sidecar")

def install(release, arch, destination):
    tag, filename = release["tag_name"], ASSETS[arch]
    assets = {a["name"]: a for a in release["assets"]}
    destination.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".telemt-", dir=destination) as directory:
        temp = Path(directory)
        for name in (filename, filename + ".sha256"):
            url = assets[name]["browser_download_url"]
            if url != PREFIX + tag + "/" + name:
                raise ValueError("Unexpected Telemt release publisher")
            fetch(url, temp / name)
        verify(temp / filename, temp / (filename + ".sha256"), assets[filename].get("digest"))
        with tarfile.open(temp / filename, "r:gz") as package:
            members = package.getmembers()
            if len(members) != 1 or members[0].name != "telemt" or not members[0].isfile() or not 0 < members[0].size <= 100 * 1024 * 1024:
                raise ValueError("Unexpected/unsafe Telemt archive")
            source = package.extractfile(members[0])
            if source is None:
                raise ValueError("Telemt executable is missing")
            with source, open(temp / "telemt", "wb") as target:
                shutil.copyfileobj(source, target, 1 << 20)
        binary = temp / "telemt"
        with open(binary, "rb") as source:
            header = source.read(20)
        if len(header) != 20 or header[:6] != b"\x7fELF\x02\x01" or struct.unpack_from("<H", header, 18)[0] != {"amd64": 62, "arm64": 183}[arch]:
            raise ValueError("Telemt executable has incorrect ELF architecture")
        binary.chmod(0o755)
        native = {"x86_64": "amd64", "aarch64": "arm64"}.get(platform.machine())
        if sys.platform == "linux" and native == arch:
            output = subprocess.check_output([str(binary), "--version"], text=True, timeout=30)
            match = re.search(r"(?i)\b(?:telemt\s+(?:version\s+)?)?v?([0-9]+\.[0-9]+\.[0-9]+(?:[-+][a-z0-9.-]+)?)", output)
            if not match or match.group(1) != tag.lstrip("v"):
                raise ValueError("Telemt binary version differs from official release")
        os.replace(binary, destination / "telemt")
    print(f"Verified official Telemt {tag} ({arch})", file=sys.stderr)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--latest-tag", action="store_true")
    parser.add_argument("--arch", choices=tuple(ASSETS))
    parser.add_argument("--tag")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    release = resolve(args.tag)
    if args.latest_tag:
        print(release["tag_name"])
    else:
        if args.arch is None or args.output is None:
            parser.error("--arch and --output are required")
        install(release, args.arch, args.output)

if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(f"Failed to fetch official Telemt: {error}", file=sys.stderr)
        sys.exit(1)
