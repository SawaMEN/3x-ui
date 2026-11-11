#!/usr/bin/env python3
"""Regression checks: panel builds must only consume the official latest stable core."""
import importlib.util
import hashlib
import io
import json
from pathlib import Path
import re
import struct
import tarfile
import tempfile
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




class ReleaseIdentityTests(unittest.TestCase):
    CORE = "a" * 40
    SING = "b" * 40
    TAG = "sawamen-" + CORE[:12] + "-" + SING[:12] + "-mask1-ciphers1-net62-sec1-stream1"

    def manifest(self):
        return {"target": {"repository": "https://github.com/SawaMEN/hiddify-core",
                           "version": self.TAG, "commit": self.CORE,
                           "submodules": {"hiddify-sing-box": self.SING}}}

    def test_accepts_release_pinned_to_both_source_gitlinks(self):
        selector.verify_release_identity({"tag_name": self.TAG, "target_commitish": self.CORE}, self.manifest())

    def test_rejects_tag_or_source_identity_mismatch(self):
        for update in ({"tag_name": "sawamen-" + "c" * 12 + "-" + self.SING[:12] + "-mask1"},
                       {"tag_name": self.TAG, "target_commitish": "d" * 40}):
            with self.subTest(update=update), self.assertRaises(ValueError):
                selector.verify_release_identity(update, self.manifest())
        modified = self.manifest()
        modified["target"]["submodules"]["hiddify-sing-box"] = "c" * 40
        with self.assertRaises(ValueError):
            selector.verify_release_identity({"tag_name": self.TAG}, modified)


class SourcePinsTest(unittest.TestCase):
    CORE = "a" * 40
    SING = "b" * 40

    def manifest(self):
        return {"target": {
            "commit": self.CORE,
            "submodules": {"hiddify-sing-box": self.SING},
        }}

    def test_exact_revision_is_accepted(self):
        selector.verify_source_pin(self.manifest(), self.CORE, self.SING)

    def test_rejects_wrong_core_or_engine_revision(self):
        for core, sing in (("c" * 40, self.SING), (self.CORE, "d" * 40)):
            with self.subTest(core=core, sing=sing), self.assertRaises(ValueError):
                selector.verify_source_pin(self.manifest(), core, sing)

    def test_rejects_missing_revision_and_malformed_pin(self):
        for manifest in ({}, {"target": {}}, {"target": {"commit": self.CORE}}):
            with self.subTest(manifest=manifest), self.assertRaises(ValueError):
                selector.verify_source_pin(manifest, self.CORE, self.SING)
        with self.assertRaises(ValueError):
            selector.verify_source_pin(self.manifest(), "../main", self.SING)
        with self.assertRaises(ValueError):
            selector.verify_source_pin(self.manifest(), self.CORE, "short")


class ExecutableFeaturesTests(unittest.TestCase):
    CORE, SING, TAG = ReleaseIdentityTests.CORE, ReleaseIdentityTests.SING, ReleaseIdentityTests.TAG
    manifest = ReleaseIdentityTests.manifest
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.output = Path(self.directory.name) / "installed"
        self.output.mkdir()
        self.binary = "hiddify-core-linux-amd64"
        (self.output / self.binary).write_bytes(b"previous-good-kernel")
        manifest = self.manifest()
        manifest["panel_build"] = {"tags": sorted(selector.TAGS | {"with_purego"})}
        elf = bytearray(20)
        elf[:6] = b"\x7fELF\x02\x01"
        struct.pack_into("<H", elf, 18, 62)
        members = {self.binary: bytes(elf), "libcronet.so": b"test-cronet", "target.json": json.dumps(manifest).encode()}
        for name in (self.binary, "libcronet.so"):
            members[name + ".sha256"] = f"{hashlib.sha256(members[name]).hexdigest()}  {name}\n".encode()
        archive = io.BytesIO()
        with tarfile.open(fileobj=archive, mode="w:gz") as package:
            for name, body in members.items():
                info = tarfile.TarInfo(name)
                info.size = len(body)
                package.addfile(info, io.BytesIO(body))
        name = "sing-box-linux-amd64.tar.gz"
        self.assets = {name: archive.getvalue()}
        self.assets[name + ".sha256"] = f"{hashlib.sha256(archive.getvalue()).hexdigest()}  {name}\n".encode()
        self.release = {"tag_name": self.TAG, "target_commitish": self.CORE, "assets": [
            {"name": name, "browser_download_url": selector.PREFIX + self.TAG + "/" + name}
            for name in self.assets
        ]}

    def install(self, tags):
        version = f"hiddify-core version {self.TAG} test\nTags: {','.join(sorted(tags))}\nRevision: {self.CORE}\n"
        def download(url, destination):
            Path(destination).write_bytes(self.assets[url.rsplit("/", 1)[-1]])
        with mock.patch.object(selector, "fetch", side_effect=download), \
             mock.patch.object(selector.subprocess, "check_output", return_value=version), \
             mock.patch.object(selector.sys, "platform", "linux"), \
             mock.patch.object(selector.platform, "machine", return_value="x86_64"):
            selector.install(self.release, "amd64", self.output, self.CORE, self.SING)

    def test_installer_rejects_binary_without_manifest_features(self):
        for tags in ((selector.TAGS - {"http2legacy"}) | {"with_purego"}, selector.TAGS):
            with self.subTest(tags=tags), self.assertRaisesRegex(ValueError, "tags|linkage"):
                self.install(tags)
            self.assertEqual((self.output / self.binary).read_bytes(), b"previous-good-kernel")

    def test_installer_accepts_binary_with_required_features(self):
        self.install(selector.TAGS | {"with_purego"})
        self.assertEqual((self.output / self.binary).read_bytes()[:4], b"\x7fELF")

    def test_requirements_match_panel_runtime_and_build_lock(self):
        root = Path(__file__).resolve().parents[1]
        process = (root / "internal/hiddify/process.go").read_text()
        block = re.search(r"var requiredBuildTags = \[\]string\{([^}]+)\}", process)
        self.assertIsNotNone(block)
        self.assertEqual(selector.TAGS, set(re.findall(r'"([^"\n]+)"', block.group(1))))
        lock = json.loads((root / "deploy/hiddify/hiddify-core-target.json").read_text())
        selector.verify_build_tags(lock["panel_build"]["tags"], "Pinned build lock")


if __name__ == "__main__":
    unittest.main()
