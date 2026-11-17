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

    def test_unknown_changes_fail_open(self):
        self.assertEqual(self.classify("LICENSE"), {
            "backend": "true", "frontend": "true"})

    def test_empty_diff_fails_open(self):
        self.assertEqual(self.classify(), {
            "backend": "true", "frontend": "true"})


if __name__ == "__main__":
    unittest.main()
