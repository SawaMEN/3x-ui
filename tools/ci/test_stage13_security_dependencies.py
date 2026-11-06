#!/usr/bin/env python3
"""Guard the kernel security manifest against stale build/audit copies."""
import json
from pathlib import Path
import runpy
import unittest

ROOT = Path(__file__).resolve().parents[2]
audit = runpy.run_path(str(ROOT / "tools/audit-hiddify-migration.py"))
validate = audit["validate_runtime_security_dependencies"]


class RuntimeSecurityPinsTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.pins = json.loads((ROOT / "docs/hiddify-core-target.json").read_text())["panel_build"]["security_dependencies"]
        cls.builder = (ROOT / "tools/build-hiddify-core.sh").read_text()

    def test_actual_manifest_and_builder_are_consistent(self):
        validate(self.pins, self.builder)

    def test_removing_any_security_module_is_rejected(self):
        for module in self.pins:
            with self.subTest(module=module), self.assertRaisesRegex(ValueError, "dependency set"):
                validate({k: v for k, v in self.pins.items() if k != module}, self.builder)

    def test_versions_below_security_floor_are_rejected(self):
        for module in self.pins:
            with self.subTest(module=module), self.assertRaisesRegex(ValueError, "Unpatched"):
                pins = dict(self.pins)
                pins[module] = "v0.0.1"
                validate(pins, self.builder)

    def test_newer_pinned_patch_is_allowed_without_duplicate_version_map(self):
        pins = dict(self.pins)
        pins["github.com/go-chi/chi/v5"] = "v5.3.1"
        validate(pins, self.builder)

    def test_builder_must_consume_manifest_and_apply_both_go_commands(self):
        for removed in ("['panel_build']['security_dependencies']", '"${security_flags[@]}"',
                        'go mod download "${security_dependencies[@]}"'):
            with self.subTest(removed=removed), self.assertRaisesRegex(ValueError, "build script"):
                validate(self.pins, self.builder.replace(removed, "deleted"))


if __name__ == "__main__":
    unittest.main()
