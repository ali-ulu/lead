import tempfile
import unittest
from pathlib import Path

import lead_hunter.db as db


class DatabaseFilterTests(unittest.TestCase):
    def setUp(self):
        self.original_db_path = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "nishan-test.db"
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

    def test_ids_filter_isolates_active_search(self):
        rows = db.list_leads({"ids": str(self.ids[0])})
        self.assertEqual([row["name"] for row in rows], ["Karachi One"])

    def test_invalid_ids_do_not_leak_all_history(self):
        rows = db.list_leads({"ids": "not-an-id"})
        self.assertEqual(rows, [])

    def test_clear_all_removes_cached_leads(self):
        db.clear_all()
        self.assertEqual(db.list_leads({}), [])


if __name__ == "__main__":
    unittest.main()
