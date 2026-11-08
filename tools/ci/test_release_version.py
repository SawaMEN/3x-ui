"""Stable tags and dev revisions must not advertise each other's identity."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("release", Path(__file__).with_name("check-release.py"))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ReleaseVersion(unittest.TestCase):
    def test_stable_tag_matches_embedded_version(self):
        release.validate_version("3.9.8\n", "0123456789", "refs/tags/v3.9.8")

    def test_stable_tag_mismatch_fails(self):
        with self.assertRaises(ValueError):
            release.validate_version("3.9.8", "0123456789", "refs/tags/v3.9.9")

    def test_prerelease_cannot_be_published_as_stable(self):
        with self.assertRaises(ValueError):
            release.validate_version("3.9.9-rc.1", "0123456789", "refs/tags/v3.9.9-rc.1")

    def test_branch_build_has_exact_revision(self):
        release.validate_version("dev+01234567\n", "0123456789", "refs/heads/update")
        with self.assertRaises(ValueError):
            release.validate_version("dev+01234567-old", "0123456789", "refs/heads/update")

    def test_branch_cannot_look_like_stable(self):
        with self.assertRaises(ValueError):
            release.validate_version("3.9.8", "0123456789", "refs/heads/main")


if __name__ == "__main__":
    unittest.main()
