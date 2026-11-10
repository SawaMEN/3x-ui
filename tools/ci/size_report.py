#!/usr/bin/env python3
"""Report the size of a DEV package and/or its embedded frontend.

Read-only by design: the bundled protocols and GeoIP/GeoSite databases are
never modified, stripped, deduplicated or downloaded by this tool.
"""
import argparse
from collections import defaultdict
import json
import os
from pathlib import Path


def list_files(directory):
    if not directory.is_dir():
        raise ValueError(f"Directory does not exist: {directory}")
    files = sorted(p for p in directory.rglob("*") if p.is_file() and not p.is_symlink())
    if not files:
        raise ValueError(f"Directory is empty: {directory}")
    return files


def package_category(relative):
    if relative == "x-ui":
        return "Panel"
    if relative.startswith("bin/hiddify-core-linux-") and not relative.endswith(".sha256"):
        return "Hiddify core"
    if relative in {"bin/telemt", "bin/mita", "bin/tuic-server"}:
        return "Bundled protocol servers"
    if relative.startswith("bin/") and relative.endswith(".dat") and (
        relative[4:].startswith("geoip") or relative[4:].startswith("geosite")
    ):
        return "GeoIP / GeoSite"
    return "Other package files"


def measure(directory, category=None):
    files = list_files(directory)
    entries = []
    totals = defaultdict(int)
    for path in files:
        relative = path.relative_to(directory).as_posix()
        size = path.stat().st_size
        group = category(relative) if category else "All files"
        totals[group] += size
        entries.append({"path": relative, "bytes": size, "category": group})
    return {
        "bytes": sum(totals.values()),
        "file_count": len(entries),
        "categories": dict(sorted(totals.items())),
        "largest_files": sorted(entries, key=lambda e: (-e["bytes"], e["path"]))[:12],
    }


def analyze(package=None, archive=None, frontend=None):
    if archive is not None and package is None:
        raise ValueError("--archive requires --package")
    if package is None and frontend is None:
        raise ValueError("Provide --package, --frontend, or both")
    result = {"schema": 1, "units": "bytes"}
    if package is not None:
        result["package"] = measure(package, package_category)
    if archive is not None:
        if not archive.is_file():
            raise ValueError(f"Archive does not exist: {archive}")
        result["archive_bytes"] = archive.stat().st_size
    if frontend is not None:
        result["frontend"] = measure(frontend, lambda p: (
            "Source maps" if p.endswith(".map") else
            "JavaScript" if p.endswith(".js") else
            "CSS" if p.endswith(".css") else
            "Other frontend files"
        ))
        if result["frontend"]["categories"].get("Source maps", 0):
            raise ValueError("Production frontend contains source maps; disable sourcemap in Vite")
    return result


def mb(size):
    return f"{size / 1_000_000:.2f} MB"


def markdown(result):
    lines = ["### Build size audit", "", "| Item | Size |", "|---|---:|"]
    if "archive_bytes" in result:
        lines.append(f"| Compressed release archive | {mb(result['archive_bytes'])} |")
    for key, title in (("package", "Installed package"), ("frontend", "Embedded frontend (uncompressed)")):
        if key not in result:
            continue
        entry = result[key]
        lines.append(f"| **{title}** | **{mb(entry['bytes'])}** |")
        for name, total in entry["categories"].items():
            lines.append(f"| └ {name} | {mb(total)} |")
    for key, title in (("package", "Largest package files"), ("frontend", "Largest frontend files")):
        if key not in result:
            continue
        lines += ["", f"<details><summary>{title}</summary>", "",
                  "| File | Size |", "|---|---:|"]
        for entry in result[key]["largest_files"]:
            lines.append(f"| `{entry['path']}` | {mb(entry['bytes'])} |")
        lines += ["", "</details>"]
    lines += ["", "Values use decimal MB; archive and unpacked sizes are not interchangeable.", ""]
    return "\n".join(lines)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--package", type=Path)
    parser.add_argument("--archive", type=Path)
    parser.add_argument("--frontend", type=Path)
    parser.add_argument("--json", type=Path, dest="json_output")
    parser.add_argument("--summary", type=Path)
    args = parser.parse_args(argv)
    try:
        result = analyze(args.package, args.archive, args.frontend)
    except ValueError as exc:
        parser.error(str(exc))
    report = markdown(result)
    print(report)
    if args.json_output:
        args.json_output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    summary = args.summary or (Path(os.environ["GITHUB_STEP_SUMMARY"]) if "GITHUB_STEP_SUMMARY" in os.environ else None)
    if summary:
        with summary.open("a", encoding="utf-8") as output:
            output.write(report + "\n")


if __name__ == "__main__":
    main()
