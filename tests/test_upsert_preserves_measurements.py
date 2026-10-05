"""Regression tests for upsert_leads preserving audit and CRM results.

A rediscovery run must never erase work that audit_lead/verify_lead recorded.
Before the fix, the ON CONFLICT DO UPDATE branch overwrote every measured
column with the NULLs carried by a discovery row.
"""
import tempfile
import unittest
from contextlib import closing
from pathlib import Path

import lead_hunter.db as db

DISCOVERED = {
    "source": "test",
    "source_id": "karachi-1",
    "name": "Karachi One",
    "country": "Pakistan",
    "city": "Karachi",
    "category": "dentist",
    "website": "https://example.com/",
    "phone": "+905000000001",
    "website_status": "unknown",
    "verification_status": "unverified",
}


class UpsertPreservesMeasurementsTests(unittest.TestCase):
    def setUp(self):
        self.original_db_path = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "leadscout-test.db"
        db.initialize()

    def tearDown(self):
        db.DB_PATH = self.original_db_path
        self.tempdir.cleanup()

    def _lead(self):
        # closing(): a bare `with connect()` only ends the transaction, it does
        # not close the handle, which keeps the temp file locked on Windows.
        with closing(db.connect()) as conn:
            row = conn.execute(
                "SELECT * FROM leads WHERE source_id='karachi-1'"
            ).fetchone()
        return dict(row)

    def _patch(self, **values):
        assignments = ",".join(f"{key}={value}" for key, value in values.items())
        with closing(db.connect()) as conn:
            conn.execute(f"UPDATE leads SET {assignments} WHERE source_id='karachi-1'")
            conn.commit()

    def test_rediscovery_keeps_visibility_scores(self):
        db.upsert_leads([dict(DISCOVERED)])
        self._patch(
            seo_score=88, aeo_score=90, geo_score=82,
            ai_visibility_score=86, opportunity_gap_score=24,
        )

        db.upsert_leads([dict(DISCOVERED)])

        row = self._lead()
        self.assertEqual(row["seo_score"], 88)
        self.assertEqual(row["aeo_score"], 90)
        self.assertEqual(row["geo_score"], 82)
        self.assertEqual(row["ai_visibility_score"], 86)
        self.assertEqual(row["opportunity_gap_score"], 24)

    def test_rediscovery_keeps_verification_and_reputation(self):
        db.upsert_leads([dict(DISCOVERED)])
        self._patch(
            verification_status="'verified'", rating=4.6,
            review_count=210, audit_engine="'heuristic'", has_https=1,
        )

        db.upsert_leads([dict(DISCOVERED)])

        row = self._lead()
        self.assertEqual(row["verification_status"], "verified")
        self.assertEqual(row["rating"], 4.6)
        self.assertEqual(row["review_count"], 210)
        self.assertEqual(row["audit_engine"], "heuristic")
        self.assertEqual(row["has_https"], 1)

    def test_rediscovery_does_not_downgrade_website_status(self):
        db.upsert_leads([dict(DISCOVERED)])
        self._patch(website_status="'weak'")

        incoming = dict(DISCOVERED)
        incoming["website_status"] = "missing"
        db.upsert_leads([incoming])

        self.assertEqual(self._lead()["website_status"], "weak")

    def test_missing_decision_is_not_reverted(self):
        nosite = dict(DISCOVERED)
        nosite["website"] = None
        nosite["website_status"] = "missing"
        db.upsert_leads([nosite])
        self.assertEqual(self._lead()["website_status"], "missing")

        again = dict(nosite)
        again["website_status"] = "unknown"
        db.upsert_leads([again])

        self.assertEqual(self._lead()["website_status"], "missing")

    def test_low_incoming_score_does_not_overwrite_high_one(self):
        db.upsert_leads([dict(DISCOVERED)])
        self._patch(seo_score=95, ai_visibility_score=93)

        incoming = dict(DISCOVERED)
        incoming["seo_score"] = 5
        incoming["ai_visibility_score"] = 3
        db.upsert_leads([incoming])

        row = self._lead()
        self.assertEqual(row["seo_score"], 95)
        self.assertEqual(row["ai_visibility_score"], 93)

    def test_crm_fields_survive_rediscovery(self):
        db.upsert_leads([dict(DISCOVERED)])
        self._patch(
            pipeline_status="'proposal'", engagement_status="'replied'",
            notes="'Discussed by phone'", follow_up_at="'2026-12-01'",
        )

        incoming = dict(DISCOVERED)
        incoming["pipeline_status"] = "new"
        incoming["engagement_status"] = "not_contacted"
        incoming["notes"] = ""
        db.upsert_leads([incoming])

        row = self._lead()
        self.assertEqual(row["pipeline_status"], "proposal")
        self.assertEqual(row["engagement_status"], "replied")
        self.assertEqual(row["notes"], "Discussed by phone")
        self.assertEqual(row["follow_up_at"], "2026-12-01")

    def test_identity_fields_still_update(self):
        db.upsert_leads([dict(DISCOVERED)])
        self._patch(seo_score=10, email="'old@example.com'")

        incoming = dict(DISCOVERED)
        incoming["phone"] = "+905555555555"
        incoming["email"] = "new@example.com"
        incoming["name"] = "Karachi One Renamed"
        db.upsert_leads([incoming])

        row = self._lead()
        self.assertEqual(row["phone"], "+905555555555")
        self.assertEqual(row["email"], "new@example.com")
        self.assertEqual(row["name"], "Karachi One Renamed")
        self.assertEqual(row["seo_score"], 10)

    def test_rediscovery_keeps_enriched_email_and_phone(self):
        # Discovery often carries no email/phone; enrich_lead fills them later.
        # A later search on the same city must not erase that work.
        db.upsert_leads([dict(DISCOVERED)])
        self._patch(email="'found@example.com'", phone="'+491234567'")

        incoming = dict(DISCOVERED)
        incoming["email"] = None
        incoming["phone"] = None
        db.upsert_leads([incoming])

        row = self._lead()
        self.assertEqual(row["email"], "found@example.com")
        self.assertEqual(row["phone"], "+491234567")

    def test_rediscovery_keeps_coordinates_when_source_omits_them(self):
        db.upsert_leads([dict(DISCOVERED)])
        self._patch(latitude=52.52, longitude=13.405)

        incoming = dict(DISCOVERED)
        incoming["latitude"] = None
        incoming["longitude"] = None
        db.upsert_leads([incoming])

        row = self._lead()
        self.assertEqual(row["latitude"], 52.52)
        self.assertEqual(row["longitude"], 13.405)

    def test_discovery_still_fills_empty_contact_fields(self):
        db.upsert_leads([dict(DISCOVERED)])
        self._patch(email="NULL", phone="NULL")

        incoming = dict(DISCOVERED)
        incoming["email"] = "new@example.com"
        incoming["phone"] = "+905000000009"
        db.upsert_leads([incoming])

        row = self._lead()
        self.assertEqual(row["email"], "new@example.com")
        self.assertEqual(row["phone"], "+905000000009")

    def test_missing_category_is_cleared_on_rediscovery(self):
        db.upsert_leads([dict(DISCOVERED)])
        self._patch(category="'wrong_category'")

        incoming = dict(DISCOVERED)
        incoming["category"] = None
        db.upsert_leads([incoming])

        self.assertIsNone(self._lead()["category"])


if __name__ == "__main__":
    unittest.main()