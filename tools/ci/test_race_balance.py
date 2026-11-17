"""Tests for profile-guided race test scheduling."""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("race-balance.py")
spec = importlib.util.spec_from_file_location("race_balance", SCRIPT)
balance = importlib.util.module_from_spec(spec)
spec.loader.exec_module(balance)


class RaceBalanceTests(unittest.TestCase):
    def test_no_profile_matches_previous_round_robin(self):
        names = [f"Test{i}" for i in range(8)]
        shards, loads, used = balance.distribute(names, 3, {})
        self.assertEqual(shards, [names[::3], names[1::3], names[2::3]])
        self.assertEqual(loads, [0.0] * 3)
        self.assertFalse(used)

    def test_balanced_makespan_and_exact_test_coverage(self):
        names = ["TestLongA", "TestSmallA", "TestSmallB", "TestLongB",
                 "TestSmallC", "TestSmallD", "TestLongC", "TestSmallE", "TestSmallF"]
        weights = {n: (30 if n.startswith("TestLong") else 1) for n in names}
        shards, loads, used = balance.distribute(names, 3, weights)
        self.assertTrue(used)
        self.assertEqual(sorted(sum(shards, [])), sorted(names))
        self.assertLess(max(loads), max(sum(weights[n] for n in names[i::3]) for i in range(3)))

    def test_unknown_tests_and_deterministic_results(self):
        names = ["TestA", "TestB", "TestC", "TestD", "TestNew", "TestOther"]
        result = balance.distribute(names, 4, {"TestA": 10, "TestC": 1})
        self.assertTrue(all(result[0]))
        self.assertEqual(sorted(sum(result[0], [])), sorted(names))
        self.assertEqual(result, balance.distribute(names, 4, {"TestA": 10, "TestC": 1}))

    def test_reject_duplicate_tests(self):
        with self.assertRaises(ValueError):
            balance.distribute(["TestX", "TestX"], 2, {})

    def test_invalid_profile_falls_back(self):
        with tempfile.TemporaryDirectory() as tmp:
            invalid = Path(tmp, "bad.json")
            invalid.write_text("broken")
            self.assertEqual(balance.load_weights(invalid, "./internal/sub"), {})

    def test_json_extraction_ignores_subtests_and_skips(self):
        with tempfile.TemporaryDirectory() as tmp:
            events = Path(tmp, "events.jsonl")
            out = Path(tmp, "shard.json")
            package = "github.com/SawaMEN/3x-ui/v3/internal/sub"
            events.write_text("\n".join(json.dumps(e) for e in [
                {"Action": "pass", "Package": package, "Test": "TestRoot", "Elapsed": 2.5},
                {"Action": "pass", "Package": package, "Test": "TestRoot/child", "Elapsed": 0.2},
                {"Action": "skip", "Package": package, "Test": "TestSkipped", "Elapsed": 0.0},
            ]))
            subprocess.run([sys.executable, str(SCRIPT), "extract", "--input", str(events),
                            "--output", str(out), "--package", "./internal/sub"], check=True)
            self.assertEqual(json.loads(out.read_text())["durations"], {"TestRoot": 2.5})

    def test_merge_smooths_durations(self):
        with tempfile.TemporaryDirectory() as tmp:
            old = Path(tmp, "previous.json")
            old.write_text(json.dumps({"schema_version": 1, "packages": {
                "./internal/sub": {"TestA": {"seconds": 10, "samples": 2}}}}))
            fragments = Path(tmp, "parts")
            fragments.mkdir()
            (fragments / "one.json").write_text(json.dumps({"schema_version": 1,
                "package": "./internal/sub", "durations": {"TestA": 5, "TestB": 3}}))
            result = Path(tmp, "out.json")
            subprocess.run([sys.executable, str(SCRIPT), "merge", "--previous", str(old),
                            "--fragments", str(fragments), "--count", "1",
                            "--output", str(result)], check=True)
            entries = json.loads(result.read_text())["packages"]["./internal/sub"]
            self.assertEqual(entries["TestA"], {"seconds": 8.0, "samples": 3})
            self.assertEqual(entries["TestB"], {"seconds": 3.0, "samples": 1})

    def test_cli_covers_each_test_once(self):
        with tempfile.TemporaryDirectory() as tmp:
            tests = Path(tmp, "names")
            names = [f"TestN{i}" for i in range(12)]
            tests.write_text("\n".join(names) + "\n")
            profile = Path(tmp, "times.json")
            profile.write_text(json.dumps({"schema_version": 1, "packages": {
                "./internal/sub": {n: {"seconds": i + 0.5} for i, n in enumerate(names)}}}))
            actual = []
            for shard in range(3):
                proc = subprocess.run([sys.executable, str(SCRIPT), "plan",
                                       "--package", "./internal/sub", "--tests", str(tests),
                                       "--profile", str(profile), "--index", str(shard),
                                       "--count", "3"], check=True, capture_output=True, text=True)
                actual.extend(proc.stdout.splitlines())
            self.assertEqual(sorted(actual), sorted(names))


if __name__ == "__main__":
    unittest.main()
