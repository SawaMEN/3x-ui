"""Exercise the real rolling-publish shell with local remotes and a fake gh."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import textwrap
import unittest

ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = (ROOT / ".github/workflows/release.yml").read_text()
PUBLISH = textwrap.dedent(WORKFLOW.split("      - name: Publish dev-latest\n", 1)[1].split("        run: |\n", 1)[1])


class RollingPublication(unittest.TestCase):
    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.work, text=True,
                                       stderr=subprocess.DEVNULL).strip()

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base_dir = Path(self.tmp.name)
        self.work = self.base_dir / "work"
        self.work.mkdir()
        self.git("init", "-q", "--initial-branch=main")
        self.git("config", "user.name", "Maintainer")
        self.git("config", "user.email", "audit@example.invalid")
        self.source = self.work / "frontend/src/example.ts"
        self.source.parent.mkdir(parents=True)
        self.source.write_text("export const value=1;\n")
        self.git("add", ".")
        self.git("commit", "-qm", "base")
        self.base = self.git("rev-parse", "HEAD")
        remote = self.base_dir / "origin.git"
        subprocess.run(["git", "init", "-q", "--bare", str(remote)], check=True)
        self.git("remote", "add", "origin", str(remote))
        self.git("push", "-q", "origin", "main")
        helper = self.work / "tools/ci/verify-format-descendant.py"
        helper.parent.mkdir(parents=True)
        shutil.copyfile(ROOT / "tools/ci/verify-format-descendant.py", helper)
        formatter = self.work / "frontend/node_modules/.bin/oxfmt"
        formatter.parent.mkdir(parents=True)
        # The fixture formatter changes whitespace only, retaining literal values.
        # Production uses the actual lockfile-pinned oxfmt installed by the job.
        formatter.write_text("#!/usr/bin/env python3\nimport pathlib,sys\np=pathlib.Path(sys.argv[-1]);p.write_text(''.join(p.read_text().split()))\n")
        formatter.chmod(0o755)
        bindir = self.base_dir / "bin"
        bindir.mkdir()
        gh = bindir / "gh"
        gh.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$TEST_GH_LOG"\n')
        gh.chmod(0o755)
        self.log = self.base_dir / "gh.log"
        self.env = dict(os.environ, PATH=str(bindir) + os.pathsep + os.environ["PATH"],
                        COMMIT=self.base, TEST_GH_LOG=str(self.log))

    def descendant(self, contents, author="Maintainer", subject="application change", path=None):
        target = path or self.source
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(contents)
        self.git("add", str(target))
        self.git("-c", "user.name=" + author, "commit", "-qm", subject)
        self.git("push", "-q", "origin", "main")

    def publish(self):
        result = subprocess.run(["bash", "-c", PUBLISH], cwd=self.work, env=self.env,
                                text=True, capture_output=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        return self.log.exists()

    def test_current_head_publishes(self):
        self.assertTrue(self.publish())
        self.assertEqual(self.git("rev-parse", "dev-latest"), self.base)
        self.assertIn("--prerelease --latest=false", self.log.read_text())

    def test_new_application_commit_does_not_publish(self):
        self.descendant("export const value=2;\n")
        self.assertFalse(self.publish())

    def test_formatter_whitespace_descendant_can_reuse_build(self):
        self.descendant("export const value = 1;\n", "github-actions[bot]", "chore(format): apply oxfmt")
        self.assertTrue(self.publish())

    def test_bot_metadata_does_not_hide_application_change(self):
        self.descendant("export const value = 2;\n", "github-actions[bot]", "chore(format): apply oxfmt")
        self.assertFalse(self.publish())

    def test_formatter_outside_frontend_does_not_publish(self):
        self.descendant("changed\n", "github-actions[bot]", "chore(format): apply oxfmt", self.work / "main.go")
        self.assertFalse(self.publish())

    def test_missing_formatter_fails_closed(self):
        self.descendant("export const value = 1;\n", "github-actions[bot]", "chore(format): apply oxfmt")
        (self.work / "frontend/node_modules/.bin/oxfmt").unlink()
        self.assertFalse(self.publish())

    def test_divergent_main_does_not_publish(self):
        self.git("checkout", "-qb", "built")
        self.source.write_text("export const value=2;\n")
        self.git("add", str(self.source))
        self.git("commit", "-qm", "built branch")
        self.env["COMMIT"] = self.git("rev-parse", "HEAD")
        self.git("checkout", "-q", "main")
        self.descendant("export const value=3;\n")
        self.assertFalse(self.publish())


if __name__ == "__main__":
    unittest.main()
