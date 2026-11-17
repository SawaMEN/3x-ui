#!/usr/bin/env python3
"""Report runner utilization and bottlenecks from actual GitHub Actions steps."""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import sys
from urllib.request import Request, urlopen


def seconds(start, finish):
    if not start or not finish:
        return 0
    a = datetime.fromisoformat(start.replace("Z", "+00:00"))
    b = datetime.fromisoformat(finish.replace("Z", "+00:00"))
    return max(0, (b - a).total_seconds())


def summary(run, jobs):
    finished = [job for job in jobs if job.get("started_at") and job.get("completed_at")]
    runtime = seconds(run.get("created_at"), run.get("updated_at"))
    active = sum(seconds(j["started_at"], j["completed_at"]) for j in finished)
    queue = sum(seconds(j.get("created_at"), j["started_at"]) for j in finished)
    rows = []
    for job in finished:
        steps = [s for s in job.get("steps", []) if s.get("started_at") and s.get("completed_at")]
        slowest = max(steps, key=lambda s: seconds(s["started_at"], s["completed_at"]), default=None)
        rows.append((
            seconds(job["started_at"], job["completed_at"]),
            job.get("name", "unknown"),
            seconds(job.get("created_at"), job["started_at"]),
            slowest,
        ))
    rows.sort(reverse=True, key=lambda item: item[0])
    lines = [
        "### GitHub Actions performance",
        "",
        f"Workflow: **{run.get('name', 'unknown')}** · run **{run.get('id', 'unknown')}**",
        "",
        f"Elapsed: **{runtime / 60:.1f} min**, sum of runner execution: "
        f"**{active / 60:.1f} runner-min**, total runner queues: **{queue / 60:.1f} min**.",
        "",
        "| Job | Execution (min) | Runner queue (min) | Slowest step (min) |",
        "|---|---:|---:|---|",
    ]
    for duration, name, wait, step in rows[:12]:
        step_label = "n/a" if step is None else (
            f"{step.get('name', 'unknown')} ({seconds(step['started_at'], step['completed_at'])/60:.1f})"
        )
        lines.append(f"| {name.replace('|', '/')} | {duration/60:.1f} | {wait/60:.1f} | {step_label.replace('|', '/')} |")
    lines.extend(["", "Compare the same workflow and runner class before/after optimization.", ""])
    return "\n".join(lines)


def fetch(url, token):
    headers = {"Accept": "application/vnd.github+json", "User-Agent": "3x-ui-ci-performance"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    with urlopen(Request(url, headers=headers), timeout=25) as response:
        return json.load(response)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", type=int, required=True)
    parser.add_argument("--repo", required=True)
    args = parser.parse_args()
    token = os.getenv("GH_TOKEN") or os.getenv("GITHUB_TOKEN")
    base = f"https://api.github.com/repos/{args.repo}/actions/runs/{args.run_id}"
    run = fetch(base, token)
    jobs = []
    page = 1
    while True:
        batch = fetch(f"{base}/jobs?per_page=100&page={page}", token)["jobs"]
        jobs.extend(batch)
        if len(batch) < 100:
            break
        page += 1
    result = summary(run, jobs)
    print(result)
    if os.getenv("GITHUB_STEP_SUMMARY"):
        with Path(os.environ["GITHUB_STEP_SUMMARY"]).open("a") as handle:
            handle.write(result + "\n")


if __name__ == "__main__":
    main()
