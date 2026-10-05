import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import lead_hunter.db as db
from lead_hunter import doctor


class DoctorTests(unittest.TestCase):
    def setUp(self):
        self.original_db_path = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "doctor-test.db"

    def tearDown(self):
        db.DB_PATH = self.original_db_path
        self.tempdir.cleanup()

    def test_report_shape_and_database_check(self):
        report = doctor.run_checks()
        self.assertIn("modules", report)
        self.assertIn("providers", report)
        self.assertTrue(report["database"]["writable"])
        self.assertIn("leads", report["database"]["tables"])
        self.assertEqual(report["database"]["lead_count"], 0)

    def test_missing_duckdb_is_reported_as_a_problem(self):
        real_find_spec = doctor.importlib.util.find_spec

        def fake_find_spec(name):
            if name == "duckdb":
                return None
            return real_find_spec(name)

        with patch.object(doctor.importlib.util, "find_spec", side_effect=fake_find_spec):
            report = doctor.run_checks()
        self.assertFalse(report["providers"]["overture"]["ok"])
        self.assertFalse(report["ok"])
        self.assertTrue(any("duckdb" in p for p in report["problems"]))
        self.assertTrue(any("Overture" in p for p in report["problems"]))

    def test_missing_module_marks_ok_false(self):
        real_find_spec = doctor.importlib.util.find_spec

        def fake_find_spec(name):
            if name == "cryptography":
                return None
            return real_find_spec(name)

        with patch.object(doctor.importlib.util, "find_spec", side_effect=fake_find_spec):
            report = doctor.run_checks()
        self.assertFalse(report["ok"])
        self.assertTrue(any("cryptography" in p for p in report["problems"]))

    def test_web_search_off_is_reported(self):
        import os
        saved = os.environ.get("LEADSCOUT_WEB_SEARCH")
        os.environ["LEADSCOUT_WEB_SEARCH"] = "off"
        try:
            report = doctor.run_checks()
        finally:
            if saved is None:
                os.environ.pop("LEADSCOUT_WEB_SEARCH", None)
            else:
                os.environ["LEADSCOUT_WEB_SEARCH"] = saved
        self.assertFalse(report["providers"]["web_search"]["ok"])
        self.assertTrue(any("Web verification" in p for p in report["problems"]))

    def test_format_report_renders_sections(self):
        text = doctor.format_report(doctor.run_checks())
        for section in ("Modules:", "Database:", "Providers:", "Config:"):
            self.assertIn(section, text)

    def test_main_exit_code_reflects_problems(self):
        import contextlib
        import io
        base = {
            "python": {"version": "3", "executable": "x"}, "node": "n",
            "modules": [{"name": "duckdb", "ok": True, "purpose": "p"}],
            "database": {"path": "/tmp/x.db", "writable": True, "tables": ["leads"], "lead_count": 0},
            "providers": {"osm": {"ok": True, "detail": "d"}},
            "config": {"LEADSCOUT_API_TOKEN": "not set"}, "categories": ["restaurant"],
        }
        with contextlib.redirect_stdout(io.StringIO()):
            with patch.object(doctor, "run_checks", return_value={**base, "ok": True, "problems": []}):
                self.assertEqual(doctor.main([]), 0)
            with patch.object(doctor, "run_checks", return_value={**base, "ok": False, "problems": ["boom"]}):
                self.assertEqual(doctor.main([]), 1)


class DegradedDiscoveryTests(unittest.TestCase):
    def setUp(self):
        self.original_db_path = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "degraded-test.db"
        db.initialize()

    def tearDown(self):
        db.DB_PATH = self.original_db_path
        self.tempdir.cleanup()

    _AREA = {
        "display_name": "Test City", "lat": 1.0, "lon": 2.0,
        "bbox": {"south": 0.9, "north": 1.1, "west": 1.9, "east": 2.1},
        "country": "Testland", "country_code": "TL", "city": "Test City",
    }

    def test_overture_failure_marks_result_degraded(self):
        from lead_hunter import services
        with patch.object(services, "geocode_area", return_value=self._AREA), \
             patch.object(services, "search_around_detailed", return_value={"rows": [], "partial": False}), \
             patch.object(services, "search_overture_bbox", side_effect=RuntimeError("duckdb missing")):
            result = services.discover_businesses(city="Test City", category="restaurant")
        self.assertTrue(result["degraded"])
        self.assertTrue(result["partial"])
        self.assertEqual(result["count"], 0)
        self.assertIn("overture", result["provider_errors"])
        self.assertTrue(any("missing data" in w for w in result["warnings"]))

    def test_unknown_provider_is_reported_and_degrades(self):
        from lead_hunter import services
        with patch.object(services, "geocode_area", return_value=self._AREA), \
             patch.object(services, "search_around_detailed", return_value={"rows": [], "partial": False}):
            result = services.discover_businesses(city="Test City", category="restaurant", providers=["osm", "typo"])
        self.assertTrue(result["degraded"])
        self.assertTrue(any("typo" in w for w in result["warnings"]))

    def test_healthy_run_is_not_degraded(self):
        from lead_hunter import services
        rows = [{
            "source": "osm", "source_id": "node:1", "name": "Test Cafe",
            "country": "Testland", "city": "Test City", "category": "restaurant",
            "latitude": 1.0, "longitude": 2.0, "website": "", "phone": "123",
            "email": "", "social_url": "", "social_links": {},
            "website_status": "missing", "data_confidence": "medium",
        }]
        with patch.object(services, "geocode_area", return_value=self._AREA), \
             patch.object(services, "search_around_detailed", return_value={"rows": rows, "partial": False}), \
             patch.object(services, "search_overture_bbox", return_value=[]):
            result = services.discover_businesses(city="Test City", category="restaurant")
        self.assertFalse(result["degraded"])
        self.assertFalse(result["partial"])
        self.assertEqual(result["count"], 1)
        self.assertEqual(result["provider_errors"], {})


if __name__ == "__main__":
    unittest.main()
