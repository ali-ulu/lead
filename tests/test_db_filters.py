import tempfile
import unittest
from pathlib import Path

import lead_hunter.db as db


class DatabaseFilterTests(unittest.TestCase):
    def setUp(self):
        self.original_db_path = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "leadscout-test.db"
        db.initialize()
        self.ids = db.upsert_leads([
            {
                "source": "test",
                "source_id": "karachi-1",
                "name": "Karachi One",
                "country": "Pakistan",
                "city": "Karachi",
                "category": "dentist",
                "website_status": "missing",
                "social_links": {},
                "data_confidence": "medium",
            },
            {
                "source": "test",
                "source_id": "berlin-1",
                "name": "Berlin One",
                "country": "Germany",
                "city": "Berlin",
                "category": "dentist",
                "website_status": "missing",
                "social_links": {},
                "data_confidence": "medium",
            },
        ])

    def tearDown(self):
        db.DB_PATH = self.original_db_path
        self.tempdir.cleanup()

    def test_ids_filter_isolates_legacy_active_search(self):
        rows = db.list_leads({"ids": str(self.ids[0])})
        self.assertEqual([row["name"] for row in rows], ["Karachi One"])

    def test_search_id_isolates_large_result_sets(self):
        search_id = db.record_search_run(
            country="Pakistan",
            city="Karachi",
            category="dentist",
            radius_km=20,
            lead_ids=[self.ids[0]],
        )
        rows = db.list_leads({"search_id": search_id})
        self.assertEqual([row["name"] for row in rows], ["Karachi One"])

    def test_unknown_search_id_returns_empty(self):
        rows = db.list_leads({"search_id": "does-not-exist"})
        self.assertEqual(rows, [])

    def test_invalid_ids_do_not_leak_all_history(self):
        rows = db.list_leads({"ids": "not-an-id"})
        self.assertEqual(rows, [])

    def test_clear_all_removes_cached_leads_and_search_runs(self):
        search_id = db.record_search_run(
            country="Pakistan",
            city="Karachi",
            category="dentist",
            radius_km=20,
            lead_ids=self.ids,
        )
        db.clear_all()
        self.assertEqual(db.list_leads({}), [])
        self.assertEqual(db.list_leads({"search_id": search_id}), [])
        with db.connect() as conn:
            self.assertEqual(conn.execute("SELECT COUNT(*) FROM search_runs").fetchone()[0], 0)
            self.assertEqual(conn.execute("SELECT COUNT(*) FROM search_run_leads").fetchone()[0], 0)


if __name__ == "__main__":
    unittest.main()
