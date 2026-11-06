#!/usr/bin/env python3
"""Fail a binary-backed Go JSON gate on missing, skipped or failed tests."""
import argparse
import json
from pathlib import Path


def verify(events, required):
    outcomes = {}
    failed = []
    for event in events:
        test = event.get("Test")
        action = event.get("Action")
        if action == "fail":
            failed.append(test or event.get("Package", "package"))
        if test and action in ("pass", "fail", "skip"):
            outcomes[test] = action
    for root in required:
        if outcomes.get(root) != "pass":
            failed.append(root + ": missing or not passed")
        failed.extend(name + ": " + result for name, result in outcomes.items()
                      if name.startswith(root + "/") and result != "pass")
    if failed:
        raise ValueError("Required Go checks did not pass: " + ", ".join(failed))
    return len(outcomes)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("log", type=Path)
    parser.add_argument("required", nargs="+")
    args = parser.parse_args()
    events = [json.loads(line) for line in args.log.read_text().splitlines() if line.strip()]
    print(f"Verified {verify(events, args.required)} test results; required roots passed without skips")


if __name__ == "__main__":
    main()
