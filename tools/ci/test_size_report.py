"""Offline regression tests for the non-destructive build size auditor."""
import importlib.util
from pathlib import Path
import tempfile
import unittest


MODULE_PATH = Path(__file__).with_name("size_report.py")
spec = importlib.util.spec_from_file_location("size_report", MODULE_PATH)
size_report = importlib.util.module_from_spec(spec)
spec.loader.exec_module(size_report)


class SizeReportTests(unittest.TestCase):
    def test_bundled_protocol_and_geo_categories(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "x-ui"
            (root / "bin").mkdir(parents=True)
            test_files = {
                "x-ui": 17,
                "bin/hiddify-core-linux-amd64": 21,
                "bin/hiddify-core-linux-amd64.sha256": 2,
                "bin/telemt": 11,
                "bin/mita": 13,
                "bin/tuic-server": 7,
                "bin/geoip.dat": 5,
                "bin/geosite.dat": 4,
                "bin/geoip_IR.dat": 3,
                "bin/geosite_IR.dat": 2,
                "bin/geoip_RU.dat": 2,
                "bin/geosite_RU.dat": 1,
            }
            for filename, size in test_files.items():
                (root / filename).write_bytes(b"x" * size)
            report = size_report.analyze(package=root)["package"]
            self.assertEqual(report["bytes"], sum(test_files.values()))
            self.assertEqual(report["file_count"], len(test_files))
            self.assertEqual(report["categories"]["GeoIP / GeoSite"], 17)
            self.assertEqual(report["categories"]["Bundled protocol servers"], 31)
            self.assertEqual(report["categories"]["Hiddify core"], 21)
            self.assertEqual(report["categories"]["Panel"], 17)

    def test_frontend_report_and_source_maps(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "assets").mkdir()
            (root / "assets/app.js").write_bytes(b"x" * 200)
            (root / "assets/main.css").write_bytes(b"x" * 50)
            report = size_report.analyze(frontend=root)
            self.assertEqual(report["frontend"]["categories"]["JavaScript"], 200)
            self.assertEqual(report["frontend"]["categories"]["CSS"], 50)
            self.assertIn("Embedded frontend", size_report.markdown(report))
            (root / "assets/app.js.map").write_bytes(b"x")
            with self.assertRaisesRegex(ValueError, "source maps"):
                size_report.analyze(frontend=root)

    def test_archive_and_json_output(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            package = root / "x-ui"
            package.mkdir()
            (package / "x-ui").write_bytes(b"x" * 32)
            archive = root / "release.tar.gz"
            archive.write_bytes(b"x" * 16)
            output = root / "size.json"
            summary = root / "summary.md"
            size_report.main(["--package", str(package), "--archive", str(archive),
                              "--json", str(output), "--summary", str(summary)])
            self.assertIn('"archive_bytes": 16', output.read_text())
            self.assertIn("Compressed release archive", summary.read_text())

    def test_empty_directory_and_invalid_archive(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            with self.assertRaisesRegex(ValueError, "empty"):
                size_report.analyze(frontend=root)
            with self.assertRaisesRegex(ValueError, "requires"):
                size_report.analyze(archive=root / "missing.tar.gz")


if __name__ == "__main__":
    unittest.main()
