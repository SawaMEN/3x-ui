#!/usr/bin/env python3
"""Regression checks: panel builds must only consume the official latest stable core."""
import importlib.util
from pathlib import Path
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "fetch_latest_kernel", Path(__file__).with_name("fetch-latest-kernel.py")
)
selector = importlib.util.module_from_spec(spec)
spec.loader.exec_module(selector)


def release(*, prerelease=False, draft=False, complete=True):
    names = [
        f"sing-box-linux-{arch}.tar.gz{suffix}"
        for arch in ("amd64", "arm64")
        for suffix in ("", ".sha256")
    ]
    if not complete:
        names.pop()
    return {
        "tag_name": "sawamen-abcdef123456-panel8",
        "published_at": "2026-10-13T00:00:00Z",
        "draft": draft,
        "prerelease": prerelease,
        "assets": [{"name": name} for name in names],
    }


class StableReleaseTests(unittest.TestCase):
    def test_accepts_complete_stable_release(self):
        self.assertTrue(selector.complete(release()))

    def test_rejects_prerelease_and_draft(self):
        self.assertFalse(selector.complete(release(prerelease=True)))
        self.assertFalse(selector.complete(release(draft=True)))
        self.assertFalse(selector.complete(release(complete=False)))

    def test_queries_official_latest_endpoint(self):
        with mock.patch.object(selector, "json_api", return_value=release()) as query:
            self.assertEqual(selector.resolve()["tag_name"], "sawamen-abcdef123456-panel8")
        query.assert_called_once_with("/releases/latest")

    def test_no_fallback_to_prerelease(self):
        with mock.patch.object(selector, "json_api", return_value=release(prerelease=True)) as query:
            with self.assertRaisesRegex(ValueError, "Latest stable kernel"):
                selector.resolve()
        query.assert_called_once_with("/releases/latest")

    def test_tagged_build_rejects_prerelease(self):
        with mock.patch.object(selector, "json_api", return_value=release(prerelease=True)):
            with self.assertRaisesRegex(ValueError, "Latest stable kernel"):
                selector.resolve("sawamen-abcdef123456-panel8")


if __name__ == "__main__":
    unittest.main()
