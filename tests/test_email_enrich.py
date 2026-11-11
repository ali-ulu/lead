"""Tests for the waterfall email enrichment provider."""
import unittest
from unittest.mock import patch

from lead_hunter.providers import email_enrich


class EmailEnrichTests(unittest.TestCase):
    def test_rejects_bare_domain(self):
        with self.assertRaises(ValueError):
            email_enrich.enrich_domain("not-a-domain")

    def test_first_provider_wins(self):
        with patch.object(email_enrich, "_hunter", return_value={"email": "a@x.com", "source": "hunter"}), \
                patch.object(email_enrich, "_pdl", side_effect=AssertionError("should not be reached")), \
                patch.object(email_enrich, "_clearbit", side_effect=AssertionError("should not be reached")):
            result = email_enrich.enrich_domain("x.com")
        self.assertTrue(result["found"])
        self.assertEqual(result["source"], "hunter")
        self.assertEqual(result["email"], "a@x.com")
        self.assertEqual([a["status"] for a in result["attempts"]], ["matched"])

    def test_falls_through_to_next_provider(self):
        with patch.object(email_enrich, "_hunter", return_value=None), \
                patch.object(email_enrich, "_pdl", return_value={"email": "b@x.com", "source": "pdl"}), \
                patch.object(email_enrich, "_clearbit", side_effect=AssertionError("should not be reached")):
            result = email_enrich.enrich_domain("x.com")
        self.assertTrue(result["found"])
        self.assertEqual(result["source"], "pdl")
        self.assertEqual([a["status"] for a in result["attempts"]], ["no_match", "matched"])

    def test_unconfigured_provider_is_recorded_as_error(self):
        with patch.object(email_enrich, "_hunter", side_effect=RuntimeError("HUNTER_API_KEY is not configured.")), \
                patch.object(email_enrich, "_pdl", return_value=None), \
                patch.object(email_enrich, "_clearbit", return_value=None):
            result = email_enrich.enrich_domain("x.com")
        self.assertFalse(result["found"])
        statuses = {a["provider"]: a["status"] for a in result["attempts"]}
        self.assertEqual(statuses["hunter"], "error")
        self.assertEqual(statuses["pdl"], "no_match")
        self.assertEqual(statuses["clearbit"], "no_match")

    def test_no_provider_matches(self):
        with patch.object(email_enrich, "_hunter", return_value=None), \
                patch.object(email_enrich, "_pdl", return_value=None), \
                patch.object(email_enrich, "_clearbit", return_value=None):
            result = email_enrich.enrich_domain("x.com")
        self.assertFalse(result["found"])
        self.assertEqual(len(result["attempts"]), 3)

    def test_strips_www_prefix(self):
        with patch.object(email_enrich, "_hunter", return_value=None), \
                patch.object(email_enrich, "_pdl", return_value=None), \
                patch.object(email_enrich, "_clearbit", return_value=None):
            result = email_enrich.enrich_domain("WWW.X.COM")
        self.assertEqual(result["domain"], "x.com")


class EnrichLeadWiringTests(unittest.TestCase):
    def setUp(self):
        import tempfile
        from pathlib import Path
        import lead_hunter.db as db

        self.db = db
        self.original_db_path = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "leadscout-enrich.db"
        db.initialize()

    def tearDown(self):
        self.db.DB_PATH = self.original_db_path
        self.tempdir.cleanup()

    def test_waterfall_runs_when_site_has_no_email(self):
        from lead_hunter import services

        [lead_id] = self.db.upsert_leads([{
            "source": "test", "source_id": "wf-1", "source_refs": {"test": "wf-1"},
            "name": "No Mail Shop", "country": "Turkey", "city": "Istanbul",
            "category": "cafe", "website": "https://nomail-shop.example",
            "website_status": "unknown", "social_links": {},
        }])
        with patch.object(services, "enrich_website", return_value={
            "pages_scanned": ["https://nomail-shop.example"],
            "emails": [], "phones": [], "social_links": {}, "booking_links": [],
            "errors": [], "complete": True,
        }), patch.object(services, "waterfall_email",
                         return_value={"found": False, "domain": "nomail-shop.example", "attempts": []}) as wf:
            payload = services.enrich_lead(lead_id)
        wf.assert_called_once_with("nomail-shop.example")
        self.assertIn("email_enrichment", payload["enrichment"])

    def test_waterfall_result_is_verified_and_saved(self):
        from lead_hunter import services

        [lead_id] = self.db.upsert_leads([{
            "source": "test", "source_id": "wf-2", "source_refs": {"test": "wf-2"},
            "name": "Found Mail Shop", "country": "Turkey", "city": "Istanbul",
            "category": "cafe", "website": "https://found-shop.example",
            "website_status": "unknown", "social_links": {},
        }])
        with patch.object(services, "enrich_website", return_value={
            "pages_scanned": ["https://found-shop.example"],
            "emails": [], "phones": [], "social_links": {}, "booking_links": [],
            "errors": [], "complete": True,
        }), patch.object(services, "waterfall_email",
                         return_value={"found": True, "email": "owner@found-shop.example", "source": "hunter", "attempts": []}), \
                patch.object(services, "check_email", return_value={"status": "valid", "score": 100}) as verify:
            payload = services.enrich_lead(lead_id)
        verify.assert_called_once()
        self.assertEqual(payload["lead"]["email"], "owner@found-shop.example")
        self.assertEqual(payload["enrichment"]["email_verification"]["status"], "valid")


if __name__ == "__main__":
    unittest.main()