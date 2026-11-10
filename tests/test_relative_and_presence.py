"""Tests for relative scoring, AI-presence measurement and visibility diffing."""
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import lead_hunter.db as db
from lead_hunter.visibility import percentile_rank, add_relative_scores
from lead_hunter import ai_presence
from lead_hunter import diff


class PercentileTests(unittest.TestCase):
    def test_rank_is_share_at_or_below(self):
        population = [10, 20, 30, 40, 50]
        self.assertEqual(percentile_rank(30, population), 60)
        self.assertEqual(percentile_rank(50, population), 100)
        self.assertEqual(percentile_rank(5, population), 0)

    def test_none_stays_none(self):
        self.assertIsNone(percentile_rank(None, [1, 2, 3]))

    def test_add_relative_scores_skips_missing(self):
        rows = [
            {"ai_visibility_score": 80},
            {"ai_visibility_score": 40},
            {"ai_visibility_score": None},
        ]
        add_relative_scores(rows)
        self.assertEqual(rows[0]["ai_visibility_score_percentile"], 100)
        self.assertEqual(rows[1]["ai_visibility_score_percentile"], 50)
        self.assertIsNone(rows[2]["ai_visibility_score_percentile"])


class LeadRelativeAnnotationTests(unittest.TestCase):
    """The HTTP endpoints must carry the percentile the UI/export expect."""

    def setUp(self):
        self.original_db = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "relative.db"
        db.initialize()
        self.ids = db.upsert_leads([
            {"source": "t", "source_id": "rel-1", "name": "Strong", "country": "US",
             "city": "NY", "category": "dentist", "ai_visibility_score": 90},
            {"source": "t", "source_id": "rel-2", "name": "Weak", "country": "US",
             "city": "NY", "category": "dentist", "ai_visibility_score": 30},
            {"source": "t", "source_id": "rel-3", "name": "Unmeasured", "country": "US",
             "city": "NY", "category": "dentist"},
        ])

    def tearDown(self):
        db.DB_PATH = self.original_db
        self.tempdir.cleanup()

    def test_list_helper_annotates_each_row(self):
        from lead_hunter import services
        rows = services.list_leads_relative({"city": "NY"})
        by_id = {r["id"]: r for r in rows}
        self.assertEqual(by_id[self.ids[0]]["ai_visibility_score_percentile"], 100)
        self.assertEqual(by_id[self.ids[1]]["ai_visibility_score_percentile"], 50)
        self.assertIsNone(by_id[self.ids[2]]["ai_visibility_score_percentile"])

    def test_single_lead_uses_its_own_market(self):
        from lead_hunter import services
        annotated = services.annotate_lead_relative(db.get_lead(self.ids[1]))
        self.assertEqual(annotated["ai_visibility_score_percentile"], 50)
        self.assertIsNone(services.annotate_lead_relative(None))
        unmeasured = services.annotate_lead_relative(db.get_lead(self.ids[2]))
        self.assertIsNone(unmeasured["ai_visibility_score_percentile"])


class AiPresenceTests(unittest.TestCase):
    def test_disabled_by_default(self):
        with mock.patch.dict(os.environ, {"LEADSCOUT_AI_PRESENCE": "0"}, clear=False):
            result = ai_presence.measure_ai_presence(name="X", category="dentist", city="Karachi")
        self.assertFalse(result["configured"])

    def test_measures_citation(self):
        results = [{"url": "https://example.test/", "title": "Example Dental", "description": "best dentist"}]
        with mock.patch.dict(os.environ, {"LEADSCOUT_AI_PRESENCE": "1"}), \
                mock.patch.object(ai_presence, "configured_providers", return_value=["duckduckgo"]), \
                mock.patch.object(ai_presence, "_run_provider", return_value=results):
            result = ai_presence.measure_ai_presence(
                name="Example Dental", category="dentist", city="Karachi", website="https://example.test/",
            )
        self.assertTrue(result["measured"])
        self.assertEqual(result["cited_count"], result["queries"])
        self.assertEqual(result["best_rank"], 1)


class VisibilityDiffTests(unittest.TestCase):
    def setUp(self):
        self.original_db = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "diff.db"
        db.initialize()

    def tearDown(self):
        db.DB_PATH = self.original_db
        self.tempdir.cleanup()

    def _lead(self, source_id, ai_visibility):
        return db.upsert_leads([{
            "source": "test", "source_id": source_id, "name": source_id,
            "country": "Pakistan", "city": "Karachi", "category": "dentist",
            "ai_visibility_score": ai_visibility,
        }])[0]

    def test_reports_visibility_improvement(self):
        lead_id = self._lead("biz-1", 30)
        first = db.record_search_run(country="Pakistan", city="Karachi", category="dentist", radius_km=10, lead_ids=[lead_id])
        db.update_lead(lead_id, {"ai_visibility_score": 72})
        second = db.record_search_run(country="Pakistan", city="Karachi", category="dentist", radius_km=10, lead_ids=[lead_id])

        report = diff.compare_runs(first, second)
        self.assertEqual(report["visibility_improved_count"], 1)
        self.assertEqual(report["visibility_improved"][0]["delta"], 42)
        text = diff.format_report(report)
        self.assertIn("Visibility improved", text)

    def test_reports_visibility_regression(self):
        lead_id = self._lead("biz-2", 80)
        first = db.record_search_run(country="Pakistan", city="Karachi", category="dentist", radius_km=10, lead_ids=[lead_id])
        db.update_lead(lead_id, {"ai_visibility_score": 55})
        second = db.record_search_run(country="Pakistan", city="Karachi", category="dentist", radius_km=10, lead_ids=[lead_id])

        report = diff.compare_runs(first, second)
        self.assertEqual(report["visibility_regressed_count"], 1)
        self.assertEqual(report["visibility_regressed"][0]["delta"], -25)


class ServicesPersistenceTests(unittest.TestCase):
    def setUp(self):
        self.original_db = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "svc.db"
        db.initialize()
        self.lead_id = db.upsert_leads([{
            "source": "test", "source_id": "svc-1", "name": "Svc Biz",
            "country": "Pakistan", "city": "Karachi", "category": "dentist",
            "website": "https://example.test/",
        }])[0]

    def tearDown(self):
        db.DB_PATH = self.original_db
        self.tempdir.cleanup()

    def test_ai_presence_is_persisted_separately(self):
        from lead_hunter import services
        with mock.patch.dict(os.environ, {"LEADSCOUT_AI_PRESENCE": "0"}, clear=False):
            payload = services.measure_ai_presence_for_lead(self.lead_id)
        self.assertFalse(payload["ai_presence"]["configured"])
        stored = db.get_lead(self.lead_id)
        self.assertIn("ai_presence", stored)
        self.assertFalse(stored["ai_presence"].get("configured", False))
        # Heuristic readiness is untouched by a measurement that did not run.
        self.assertIsNone(stored["seo_score"])


if __name__ == "__main__":
    unittest.main()
