#!/usr/bin/env python3
"""Deterministic Go -race sharding using observed per-test durations."""
import argparse
import json
import math
from pathlib import Path
import re
import statistics
import sys

TEST_NAME = re.compile(r"^(?:Test|Example|Fuzz)[A-Za-z0-9_]+$")
SCHEMA = 1


def read_json(path):
    try:
        return json.loads(Path(path).read_text())
    except (OSError, UnicodeError, json.JSONDecodeError):
        return None


def weight(value):
    try:
        seconds = float(value)
    except (ValueError, TypeError):
        return None
    return seconds if math.isfinite(seconds) and seconds > 0 else None


def load_weights(path, package):
    data = read_json(path)
    if not isinstance(data, dict) or data.get("schema_version") != SCHEMA:
        return {}
    packages = data.get("packages", {})
    if not isinstance(packages, dict):
        return {}
    records = packages.get(package, {})
    if not isinstance(records, dict):
        return {}
    result = {}
    for name, entry in records.items():
        if not isinstance(name, str) or not TEST_NAME.fullmatch(name) or not isinstance(entry, dict):
            continue
        seconds = weight(entry.get("seconds"))
        if seconds is not None:
            result[name] = seconds
    return result


def distribute(names, count, durations):
    """Longest-processing-time-first, with the old round-robin fallback."""
    if count <= 0 or len(names) < count or len(set(names)) != len(names):
        raise ValueError("invalid test set / shard count")
    buckets = [[] for _ in range(count)]
    loads = [0.0] * count
    observed = [durations[name] for name in names if name in durations]
    if not observed:
        for position, name in enumerate(names):
            buckets[position % count].append(name)
        return buckets, loads, False
    unknown_weight = max(0.01, statistics.median(observed))
    ordered = sorted(names, key=lambda name: (-durations.get(name, unknown_weight), name))
    for name in ordered:
        index = min(range(count), key=lambda i: (loads[i], len(buckets[i]), i))
        buckets[index].append(name)
        loads[index] += durations.get(name, unknown_weight)
    return buckets, loads, True


def plan(args):
    names = [line.strip() for line in Path(args.tests).read_text().splitlines()
             if TEST_NAME.fullmatch(line.strip())]
    buckets, loads, balanced = distribute(names, args.count, load_weights(args.profile, args.package))
    print("\n".join(buckets[args.index]))
    if balanced:
        print(f"Weighted race shards, estimated seconds: {[round(v, 1) for v in loads]}",
              file=sys.stderr)
    else:
        print(f"No race timings; preserving round-robin over {len(names)} tests", file=sys.stderr)


def extract(args):
    result = {}
    expected = "/" + args.package.removeprefix("./").rstrip("/")
    for raw in Path(args.input).read_text().splitlines():
        try:
            event = json.loads(raw)
        except json.JSONDecodeError:
            continue
        name = event.get("Test")
        if (event.get("Action") != "pass" or not isinstance(name, str)
                or not TEST_NAME.fullmatch(name)
                or not str(event.get("Package", "")).endswith(expected)):
            continue
        elapsed = weight(event.get("Elapsed"))
        if elapsed is not None:
            result[name] = max(0.01, elapsed)
    if not result:
        raise ValueError("no passing top-level test durations found in Go JSON events")
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    Path(args.output).write_text(json.dumps({"schema_version": SCHEMA, "package": args.package,
                                               "durations": result}, sort_keys=True, indent=2) + "\n")
    print(f"Measured {len(result)} top-level tests in {args.package}", file=sys.stderr)


def merge(args):
    previous = read_json(args.previous)
    if not isinstance(previous, dict) or previous.get("schema_version") != SCHEMA:
        previous = {}
    packages = previous.get("packages", {})
    if not isinstance(packages, dict):
        packages = {}
    fragments = sorted(Path(args.fragments).glob("*.json"))
    if len(fragments) != args.count:
        raise ValueError(f"expected {args.count} race shards, got {len(fragments)}")
    updated = 0
    for fragment in fragments:
        data = read_json(fragment)
        if not isinstance(data, dict) or data.get("schema_version") != SCHEMA:
            raise ValueError(f"invalid race fragment: {fragment}")
        package = data.get("package")
        if package not in ("./internal/web/service", "./internal/sub"):
            raise ValueError(f"unknown package in {fragment}")
        observed = data.get("durations")
        if not isinstance(observed, dict) or not observed:
            raise ValueError(f"empty fragment: {fragment}")
        records = packages.setdefault(package, {})
        if not isinstance(records, dict):
            records = packages[package] = {}
        for name, value in observed.items():
            seconds = weight(value)
            if not isinstance(name, str) or not TEST_NAME.fullmatch(name) or seconds is None:
                raise ValueError(f"invalid test timing {name}")
            old = records.get(name, {})
            old_seconds = weight(old.get("seconds")) if isinstance(old, dict) else None
            samples = old.get("samples", 0) if isinstance(old, dict) else 0
            if not isinstance(samples, int) or samples < 0:
                samples = 0
            smoothed = seconds if old_seconds is None else 0.6 * old_seconds + 0.4 * seconds
            records[name] = {"seconds": round(smoothed, 5), "samples": min(samples + 1, 1000)}
            updated += 1
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    Path(args.output).write_text(json.dumps({"schema_version": SCHEMA, "packages": packages},
                                            sort_keys=True, indent=2) + "\n")
    print(f"Saved {updated} durations from {len(fragments)} shards")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_subparsers(dest="command", required=True)
    p = actions.add_parser("plan")
    p.add_argument("--package", required=True)
    p.add_argument("--tests", required=True)
    p.add_argument("--profile", required=True)
    p.add_argument("--index", type=int, required=True)
    p.add_argument("--count", type=int, required=True)
    p.set_defaults(func=plan)
    p = actions.add_parser("extract")
    p.add_argument("--package", required=True)
    p.add_argument("--input", required=True)
    p.add_argument("--output", required=True)
    p.set_defaults(func=extract)
    p = actions.add_parser("merge")
    p.add_argument("--previous", required=True)
    p.add_argument("--fragments", required=True)
    p.add_argument("--output", required=True)
    p.add_argument("--count", type=int, required=True)
    p.set_defaults(func=merge)
    args = parser.parse_args()
    if args.command == "plan" and not 0 <= args.index < args.count:
        parser.error("index must be within shard count")
    try:
        args.func(args)
    except (OSError, ValueError) as error:
        parser.error(str(error))


if __name__ == "__main__":
    main()
