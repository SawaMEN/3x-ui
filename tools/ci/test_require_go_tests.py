"""The release gate must reject false greens, not just parse JSON."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("gate", Path(__file__).with_name("require-go-tests.py"))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class RequiredChecks(unittest.TestCase):
    def test_valid_root_and_children(self):
        self.assertEqual(gate.verify([{"Action": "pass", "Test": "TestNative/case"},
                                      {"Action": "pass", "Test": "TestNative"}], ["TestNative"]), 2)

    def test_missing_root_is_not_success(self):
        with self.assertRaises(ValueError):
            gate.verify([{"Action": "pass", "Package": "example"}], ["TestNative"])

    def test_skipped_subtest_is_not_success(self):
        with self.assertRaises(ValueError):
            gate.verify([{"Action": "skip", "Test": "TestNative/case"},
                         {"Action": "pass", "Test": "TestNative"}], ["TestNative"])

    def test_failure_is_not_success(self):
        with self.assertRaises(ValueError):
            gate.verify([{"Action": "pass", "Test": "TestNative"},
                         {"Action": "fail", "Package": "example"}], ["TestNative"])


if __name__ == "__main__":
    unittest.main()
