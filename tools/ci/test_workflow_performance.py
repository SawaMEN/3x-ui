"""Tests for GitHub Actions workflow timing summaries."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("workflow_perf", Path(__file__).with_name("workflow-performance.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class PerformanceSummaryTests(unittest.TestCase):
    def test_actual_job_and_queue_times_are_distinct(self):
        run = {
            "id": 12, "name": "CI",
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:09:00Z",
        }
        jobs = [{
            "name": "Go test", "created_at": "2026-01-01T00:01:00Z",
            "started_at": "2026-01-01T00:04:00Z",
            "completed_at": "2026-01-01T00:09:00Z",
            "steps": [{"name": "Tests", "started_at": "2026-01-01T00:04:00Z",
                       "completed_at": "2026-01-01T00:08:00Z"}],
        }]
        text = module.summary(run, jobs)
        self.assertIn("9.0 min", text)
        self.assertIn("5.0 runner-min", text)
        self.assertIn("3.0 min", text)
        self.assertIn("Tests (4.0)", text)

    def test_missing_step_timestamps_do_not_crash(self):
        run = {"created_at": "2026-01-01T00:00:00Z", "updated_at": None}
        jobs = [{"name": "waiting", "started_at": None, "completed_at": None}]
        self.assertIn("0.0 min", module.summary(run, jobs))


if __name__ == "__main__":
    unittest.main()
