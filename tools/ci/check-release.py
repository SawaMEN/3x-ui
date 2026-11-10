#!/usr/bin/env python3
"""Validate the assembled Linux package and emit a per-file digest manifest."""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import stat
import struct
import subprocess


def validate_version(version, revision, ref):
    version = version.strip()
    if ref.startswith("refs/tags/"):
        tag = ref.removeprefix("refs/tags/")
        if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag):
            raise ValueError("Stable publication requires a numeric stable version tag")
        if version != tag[1:]:
            raise ValueError("Panel version differs from the stable release tag")
    elif version != "dev+" + revision[:8]:
        raise ValueError("Panel version does not advertise the exact development revision")


def validate_elf_linkage(name, arch, programs, dynamic, cronet_linkage):
    """Allow only the reviewed glibc loader for purego Cronet kernels.

    Purego uses dlopen/dlsym through glibc, so CGO_ENABLED=0 does not imply
    a static ELF. The x-ui executable remains fully static. Unknown loaders,
    extra shared libraries and RPATH/RUNPATH are still release blockers.
    """
    loader = re.findall(r"\[Requesting program interpreter: ([^\]]+)\]", programs)
    libraries = re.findall(r"\(NEEDED\)\s+Shared library: \[([^\]]+)\]", dynamic)
    interp_present = bool(re.search(r"^\s*INTERP\s", programs, re.MULTILINE))
    if not loader and not libraries and not interp_present and "(NEEDED)" not in dynamic:
        return
    approved_loader = {
        "amd64": "/lib64/ld-linux-x86-64.so.2",
        "arm64": "/lib/ld-linux-aarch64.so.1",
    }[arch]
    approved_libraries = {"libc.so.6", "libpthread.so.0", "libdl.so.2"}
    if (name != f"bin/hiddify-core-linux-{arch}"
            or cronet_linkage != "purego"
            or not interp_present or loader != [approved_loader]
            or len(libraries) != len(approved_libraries)
            or set(libraries) != approved_libraries
            or "(RPATH)" in dynamic or "(RUNPATH)" in dynamic):
        raise ValueError(
            f"Unapproved ELF runtime linkage: {name}; "
            f"INTERP={loader}; NEEDED={libraries}; Cronet={cronet_linkage}")


def check(root, arch, revision):
    root = root.resolve()
    machine = {"amd64": 62, "arm64": 183}[arch]
    core = f"bin/hiddify-core-linux-{arch}"
    binaries = ["x-ui", core, "bin/telemt", "bin/mita", "bin/tuic-server"]
    scripts = ["x-ui.sh", "x-ui-core.sh", "telemt-update.sh", "hiddify-core-update.sh"]
    required = binaries + scripts + ["core-migration.sh", "fetch-latest-kernel.py", "hiddify-core-update.service", "hiddify-core-update.timer", "bin/target.json", "bin/libcronet.so", "bin/libcronet.so.sha256", "bin/speed-components.json",
                                   "x-ui.service.debian", "x-ui.service.arch", "x-ui.service.rhel",
                                   "bin/geoip.dat", "bin/geosite.dat",
                                   "bin/geoip_IR.dat", "bin/geosite_IR.dat",
                                   "bin/geoip_RU.dat", "bin/geosite_RU.dat"]
    for name in required:
        path = root / name
        if not path.is_file() or path.stat().st_size == 0:
            raise ValueError("Missing/empty package member: " + name)
    for name in binaries:
        with (root / name).open("rb") as source:
            header = source.read(20)
        if header[:6] != b"\x7fELF\x02\x01" or struct.unpack_from("<H", header, 18)[0] != machine:
            raise ValueError("Incorrect ELF architecture: " + name)
    for name in binaries + scripts:
        if not (root / name).stat().st_mode & stat.S_IXUSR:
            raise ValueError("Missing executable bit: " + name)
    # Purego kernels load glibc for dlopen and use the separately verified,
    # architecture-matched Cronet shared library bundled in the package.
    with (root / "bin/libcronet.so").open("rb") as source:
        header = source.read(20)
    if (len(header) < 20 or header[:6] != b"\x7fELF\x02\x01"
            or struct.unpack_from("<HH", header, 16) != (3, machine)):
        raise ValueError("Cronet shared library has incorrect ELF identity")
    metadata = subprocess.check_output(["go", "version", "-m", str(root / "x-ui")], text=True)
    if f"vcs.revision={revision}" not in metadata or "vcs.modified=false" not in metadata:
        raise ValueError("Panel build provenance is missing, dirty or differs from the release SHA")
    actual = json.loads((root / "bin/target.json").read_text())
    for name in ("x-ui", core):
        programs = subprocess.check_output(["readelf", "-l", str(root / name)], text=True)
        dynamic = subprocess.check_output(["readelf", "-d", str(root / name)], text=True)
        validate_elf_linkage(name, arch, programs, dynamic, actual.get("panel_build", {}).get("cronet_linkage"))
    panel_version = subprocess.check_output([str(root / "x-ui"), "-v"], text=True, timeout=30)
    validate_version(panel_version, revision, os.environ.get("GITHUB_REF", ""))
    expected = json.loads(Path("deploy/hiddify/hiddify-core-target.json").read_text())
    # The panel tracks the latest published stable core, not the source SHA
    # previously frozen in its historical build lock. Preserve compatibility
    # by requiring the same feature set, interface, transport profile and
    # security linkage. Version/commit are checked against the release itself.
    target, profile = actual["target"], actual["panel_build"]
    old_profile = expected["target"]["version"].split("-mask", 1)
    new_profile = target["version"].split("-mask", 1)
    selected_tag = os.environ.get("CORE_RELEASE_TAG", "")
    if (not selected_tag or target["version"] != selected_tag
            or len(old_profile) != 2 or len(new_profile) != 2
            or old_profile[1] != new_profile[1]
            or target["repository"] != "https://github.com/SawaMEN/hiddify-core"
            or not re.fullmatch(r"[a-f0-9]{40}", target["commit"])
            or not re.fullmatch(r"[a-f0-9]{40}", target["submodules"]["hiddify-sing-box"])
            or not target["version"].startswith(
                "sawamen-" + target["commit"][:12] + "-" + target["submodules"]["hiddify-sing-box"][:12] + "-")
            or not set(expected["panel_build"]["tags"]).issubset(profile["tags"])
            or profile.get("cronet_linkage") != "purego"
            or profile.get("runtime_dependencies") != expected["panel_build"].get("runtime_dependencies")
            or actual.get("runtime_contract", {}).get("stats_grpc_method")
                  != expected["runtime_contract"]["stats_grpc_method"]):
        raise ValueError("Latest core release violates panel's mandatory runtime/ABI contract")
    for name in (core, "bin/libcronet.so"):
        fields = (root / (name + ".sha256")).read_text().split()
        actual_digest = hashlib.file_digest((root / name).open("rb"), "sha256").hexdigest()
        if len(fields) != 2 or fields[0] != actual_digest or fields[1].lstrip("*") != Path(name).name:
            raise ValueError("Core/runtime checksum mismatch: " + name)
    files = []
    for path in sorted(root.rglob("*")):
        if path.is_symlink():
            raise ValueError("Unexpected package symlink: " + str(path))
        if path.is_file() and path.name != "manifest.json":
            files.append({"path": str(path.relative_to(root)), "mode": oct(stat.S_IMODE(path.stat().st_mode)),
                          "size": path.stat().st_size, "sha256": hashlib.file_digest(path.open("rb"), "sha256").hexdigest()})
    return {"schema": 1, "revision": revision, "arch": arch, "panel_build_info": metadata, "panel_version": panel_version.strip(),
            "core_target": actual["target"], "files": files}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("package", type=Path)
    parser.add_argument("arch", choices=["amd64", "arm64"])
    parser.add_argument("revision")
    args = parser.parse_args()
    result = check(args.package, args.arch, args.revision)
    (args.package / "manifest.json").write_text(json.dumps(result, indent=2) + "\n")
    print(f"Validated {len(result['files'])} package files for {args.arch}")


if __name__ == "__main__":
    main()
