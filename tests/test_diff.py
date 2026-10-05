import tempfile
import unittest
from pathlib import Path

import lead_hunter.db as db
from lead_hunter import diff


class RunDiffTests(unittest.TestCase):
    def setUp(self):
        self.original_db = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "diff.db"
        db.initialize()

    def tearDown(self):
        db.DB_PATH = self.original_db
        self.tempdir.cleanup()

    def _lead(self, source_id, name, *, score=80, website_status="missing", phone="555", website=None):
        [lead_id] = db.upsert_leads([{
            "source": "test", "source_id": source_id, "name": name,
            "country": "TR", "city": "Afyonkarahisar", "category": "restaurant",
            "website_status": website_status, "website": website, "phone": phone,
        }])
        if score:
            # Scoring is covered by its own tests; force the value here so the
            # diff assertions are about the diff, not about score weights.
            with db.connect() as conn:
                conn.execute("UPDATE leads SET lead_score=? WHERE id=?", (int(score), lead_id))
                conn.commit()
        return lead_id

    def _run(self, lead_ids):
        return db.record_search_run(
            country="TR", city="Afyonkarahisar", category="restaurant",
            radius_km=15, lead_ids=lead_ids,
        )

    def test_detects_new_and_gone_leads(self):
        a = self._lead("a", "Alpha")
        first = self._run([a])
        b = self._lead("b", "Beta")
        second = self._run([b])

        report = diff.compare_runs(first, second)
        self.assertEqual(report["new_count"], 1)
        self.assertEqual(report["gone_count"], 1)
        self.assertEqual(report["new"][0]["name"], "Beta")
        self.assertEqual(report["gone"][0]["name"], "Alpha")

    def test_detects_website_gained_and_lost(self):
        lead = self._lead("a", "Alpha", website_status="missing")
        first = self._run([lead])
        db.update_lead(lead, {"website_status": "healthy", "website": "https://alpha.example"})
        second = self._run([lead])

        gained = diff.compare_runs(first, second)
        self.assertEqual(gained["website_gained_count"], 1)
        self.assertEqual(gained["website_gained"][0]["name"], "Alpha")

        db.update_lead(lead, {"website_status": "missing", "website": ""})
        third = self._run([lead])
        lost = diff.compare_runs(second, third)
        self.assertEqual(lost["website_lost_count"], 1)

    def test_high_opportunity_alert_needs_score_missing_site_and_phone(self):
        # score 80, missing site, phone present -> alert
        good = self._lead("good", "Good Lead")
        # no phone -> not an alert
        no_phone = self._lead("np", "No Phone", phone="")
        first = self._run([])
        second = self._run([good, no_phone])

        report = diff.compare_runs(first, second)
        names = {item["name"] for item in report["alerts"]}
        self.assertIn("Good Lead", names)
        self.assertNotIn("No Phone", names)

    def test_alerts_only_fire_for_new_leads(self):
        lead = self._lead("a", "Existing")
        first = self._run([lead])
        second = self._run([lead])
        report = diff.compare_runs(first, second)
        # A qualifying lead that was already present must not re-alert nightly.
        self.assertEqual(report["alert_count"], 0)
        self.assertEqual(report["new_count"], 0)

    def test_requires_both_ids(self):
        with self.assertRaises(ValueError):
            diff.compare_runs("", "x")
        with self.assertRaises(ValueError):
            diff.compare_runs("x", "")

    def test_format_report_renders_sections(self):
        a = self._lead("a", "Alpha")
        first = self._run([a])
        b = self._lead("b", "Beta")
        second = self._run([b])
        text = diff.format_report(diff.compare_runs(first, second))
        self.assertIn("Run diff:", text)
        self.assertIn("New businesses", text)
        self.assertIn("No longer seen", text)


class NightlyScriptTests(unittest.TestCase):
    def setUp(self):
        self.original_db = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "nightly.db"
        db.initialize()
        self.out_dir = Path(self.tempdir.name) / "exports"

    def tearDown(self):
        db.DB_PATH = self.original_db
        self.tempdir.cleanup()

    def _fake_discover(self, calls):
        from lead_hunter import services

        def fake(*, city, category, country="", radius_km=20, **kwargs):
            calls.append((city, category))
            [lead_id] = db.upsert_leads([{
                "source": "test", "source_id": f"{city}-{category}-{len(calls)}",
                "name": f"{city} {category} {len(calls)}", "country": country,
                "city": city, "category": category, "website_status": "missing", "phone": "555",
            }])
            search_id = db.record_search_run(
                country=country, city=city, category=category, radius_km=radius_km, lead_ids=[lead_id],
            )
            return {"ok": True, "count": 1, "search_id": search_id, "degraded": False, "warnings": []}

        return fake

    def test_nightly_runs_each_job_and_writes_report(self):
        import scripts.nightly as nightly
        from unittest.mock import patch
        calls = []
        with patch("lead_hunter.services.discover_businesses", side_effect=self._fake_discover(calls)):
            report = nightly.run_nightly(
                [
                    {"city": "Afyonkarahisar", "country": "Turkey", "category": "restaurant"},
                    {"city": "Afyonkarahisar", "country": "Turkey", "category": "dentist"},
                ],
                out_dir=self.out_dir,
            )
        self.assertEqual(report["job_count"], 2)
        self.assertEqual(len(calls), 2)
        self.assertEqual(report["total_leads"], 2)
        self.assertTrue(Path(report["report_path"]).exists())
        self.assertTrue(Path(report["summary_path"]).exists())
        self.assertTrue(Path(report["export_path"]).exists())

    def test_second_nightly_produces_a_diff(self):
        import scripts.nightly as nightly
        from unittest.mock import patch
        calls = []
        with patch("lead_hunter.services.discover_businesses", side_effect=self._fake_discover(calls)):
            nightly.run_nightly([{"city": "Afyonkarahisar", "category": "restaurant"}], out_dir=self.out_dir)
            report = nightly.run_nightly([{"city": "Afyonkarahisar", "category": "restaurant"}], out_dir=self.out_dir)
        self.assertTrue(report["results"][0]["diff"])
        self.assertEqual(report["results"][0]["diff"]["new_count"], 1)

    def test_unknown_category_is_reported_not_crashed(self):
        import scripts.nightly as nightly
        report = nightly.run_nightly([{"city": "Afyonkarahisar", "category": "not_a_category"}], out_dir=self.out_dir)
        self.assertEqual(len(report["errors"]), 1)
        self.assertIn("unsupported category", report["errors"][0]["error"])

    def test_empty_jobs_raise(self):
        import scripts.nightly as nightly
        with self.assertRaises(ValueError):
            nightly.run_nightly([], out_dir=self.out_dir)


if __name__ == "__main__":
    unittest.main()
