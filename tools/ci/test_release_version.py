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


class ReleaseLinkage(unittest.TestCase):
    _amd64_program = ("  INTERP         0x0000000000000fe4\n"
                      "      [Requesting program interpreter: /lib64/ld-linux-x86-64.so.2]\n")
    _arm64_program = ("  INTERP         0x0000000000000fe4\n"
                      "      [Requesting program interpreter: /lib/ld-linux-aarch64.so.1]\n")
    _glibc = "\n".join(
        f" 0x0000000000000001 (NEEDED) Shared library: [{library}]"
        for library in ("libdl.so.2", "libpthread.so.0", "libc.so.6")
    )

    def test_static_panel_remains_required(self):
        release.validate_elf_linkage("x-ui", "amd64", "", "There is no dynamic section", "purego")
        with self.assertRaises(ValueError):
            release.validate_elf_linkage("x-ui", "amd64", self._amd64_program, self._glibc, "purego")

    def test_approved_purego_core_linkage(self):
        release.validate_elf_linkage("bin/hiddify-core-linux-amd64", "amd64",
                                     self._amd64_program, self._glibc, "purego")
        release.validate_elf_linkage("bin/hiddify-core-linux-arm64", "arm64",
                                     self._arm64_program, self._glibc, "purego")

    def test_disallow_wrong_loader_and_extra_library(self):
        with self.assertRaises(ValueError):
            release.validate_elf_linkage("bin/hiddify-core-linux-amd64", "amd64",
                                         self._arm64_program, self._glibc, "purego")
        with self.assertRaises(ValueError):
            release.validate_elf_linkage("bin/hiddify-core-linux-amd64", "amd64",
                                         self._amd64_program, self._glibc + "\n 0x1 (NEEDED) Shared library: [libcurl.so.4]", "purego")

    def test_disallow_glibc_in_musl_profile_or_rpath(self):
        with self.assertRaises(ValueError):
            release.validate_elf_linkage("bin/hiddify-core-linux-amd64", "amd64",
                                         self._amd64_program, self._glibc, "musl")
        with self.assertRaises(ValueError):
            release.validate_elf_linkage("bin/hiddify-core-linux-amd64", "amd64",
                                         self._amd64_program, self._glibc + "\n(RUNPATH)", "purego")


if __name__ == "__main__":
    unittest.main()
