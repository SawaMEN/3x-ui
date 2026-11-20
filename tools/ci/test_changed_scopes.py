"""CI scope selection must never silently ignore unknown source changes."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("changed-scopes.sh")


class ScopeTests(unittest.TestCase):
    def classify(self, *paths):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp, "paths.txt")
            path.write_text("\n".join(paths) + "\n")
            env = dict(os.environ, GITHUB_OUTPUT="/dev/stdout")
            result = subprocess.run(
                ["bash", str(SCRIPT), str(path)], check=True,
                capture_output=True, text=True, env=env,
            )
        return dict(line.split("=", 1) for line in result.stdout.splitlines())

    def test_frontend_only(self):
        self.assertEqual(self.classify("frontend/src/App.tsx"), {
            "backend": "false", "frontend": "true"})

    def test_go_only(self):
        self.assertEqual(self.classify("internal/database/store.go", "go.mod"), {
            "backend": "true", "frontend": "false"})

    def test_mixed_changes(self):
        self.assertEqual(self.classify("internal/web/service/api.go", "frontend/package-lock.json"), {
            "backend": "true", "frontend": "true"})

    def test_ci_workflow_changes_check_both_suites(self):
        self.assertEqual(self.classify(".github/workflows/ci.yml"), {
            "backend": "true", "frontend": "true"})

    def test_kernel_fetcher_and_regression_tests_trigger_ci(self):
        workflow = (SCRIPT.parents[2] / ".github/workflows/ci.yml").read_text()
        event_paths = workflow.split("\npermissions:", 1)[0]
        for path in ("tools/fetch-latest-kernel.py", "tools/test_fetch_latest_kernel.py"):
            with self.subTest(path=path):
                self.assertEqual(event_paths.count(f'      - "{path}"'), 2)
                self.assertEqual(self.classify(path), {"backend": "true", "frontend": "false"})

    def test_unknown_changes_fail_open(self):
        self.assertEqual(self.classify("LICENSE"), {
            "backend": "true", "frontend": "true"})

    def test_empty_diff_fails_open(self):
        self.assertEqual(self.classify(), {
            "backend": "true", "frontend": "true"})


if __name__ == "__main__":
    unittest.main()
