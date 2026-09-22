import tempfile
import unittest
from pathlib import Path

import lead_hunter.db as db
from lead_hunter.audit import SignalParser, _structured_data
from lead_hunter.visibility import calculate_visibility, missing_website_visibility


class VisibilityScoreTests(unittest.TestCase):
    def test_strong_page_scores_high(self):
        result = calculate_visibility({
            "indexable": True,
            "title": "Example Dental Karachi",
            "meta_description": True,
            "h1_count": 1,
            "h2_h3_count": 6,
            "canonical": "https://example.com/",
            "has_https": True,
            "mobile_ok": True,
            "performance_score": 88,
            "lighthouse_seo_score": 95,
            "word_count": 950,
            "question_headings": 4,
            "business_schema": True,
            "organization_schema": False,
            "schema_types": ["Dentist", "LocalBusiness"],
            "schema_fields": ["name", "address", "telephone", "url", "logo", "sameAs"],
            "external_links": 4,
            "fact_signals": 12,
            "author_signal": True,
            "fresh_signal": True,
            "semantic_main": True,
            "social_count": 3,
            "contact_signal": True,
        }, commercial_score=80)
        self.assertGreaterEqual(result["seo_score"], 85)
        self.assertGreaterEqual(result["aeo_score"], 80)
        self.assertGreaterEqual(result["geo_score"], 80)
        self.assertGreaterEqual(result["ai_visibility_score"], 80)
        self.assertLess(result["opportunity_gap_score"], 50)

    def test_noindex_penalizes_ai_visibility(self):
        evidence = {
            "indexable": False,
            "title": "Strong page",
            "meta_description": True,
            "h1_count": 1,
            "h2_h3_count": 5,
            "canonical": "https://example.com/",
            "has_https": True,
            "mobile_ok": True,
            "performance_score": 90,
            "lighthouse_seo_score": 95,
            "word_count": 900,
            "question_headings": 3,
            "business_schema": True,
            "schema_fields": ["name", "address", "telephone", "url", "sameAs"],
            "external_links": 3,
            "fact_signals": 8,
            "author_signal": True,
            "fresh_signal": True,
            "semantic_main": True,
            "social_count": 2,
            "contact_signal": True,
        }
        result = calculate_visibility(evidence)
        self.assertLess(result["ai_visibility_score"], 60)

    def test_verified_missing_site_has_max_owned_web_gap(self):
        result = missing_website_visibility(commercial_score=80, verified=True)
        self.assertEqual(result["seo_score"], 0)
        self.assertEqual(result["ai_visibility_score"], 0)
        self.assertGreaterEqual(result["opportunity_gap_score"], 90)

    def test_jsonld_business_signals_are_extracted(self):
        parser = SignalParser()
        parser.feed("""<html><head>
        <script type="application/ld+json">
        {"@context":"https://schema.org","@type":"Dentist","name":"Example Dental",
         "address":{"@type":"PostalAddress","addressLocality":"Karachi"},
         "telephone":"+92 21 0000000","url":"https://example.test","sameAs":["https://instagram.com/example"]}
        </script></head><body><main><h1>Example Dental</h1><h2>How can we help?</h2></main></body></html>""")
        types, fields = _structured_data(parser.jsonld_blocks)
        self.assertIn("Dentist", types)
        self.assertIn("address", fields)
        self.assertIn("sameAs", fields)
        self.assertTrue(parser.semantic_main)
        self.assertIn("How can we help?", parser.heading_texts)


class VisibilityDatabaseTests(unittest.TestCase):
    def setUp(self):
        self.original_db = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "visibility.db"
        db.initialize()

    def tearDown(self):
        db.DB_PATH = self.original_db
        self.tempdir.cleanup()

    def test_opportunity_gap_is_persisted_and_filterable(self):
        [lead_id] = db.upsert_leads([{
            "source": "test",
            "source_id": "gap-1",
            "name": "Gap Business",
            "country": "Pakistan",
            "city": "Karachi",
            "category": "dentist",
            "website": "https://example.test",
            "website_status": "weak",
            "seo_score": 30,
            "aeo_score": 20,
            "geo_score": 15,
            "ai_visibility_score": 22,
            "opportunity_gap_score": 86,
            "visibility_reasons": {"seo": ["weak"]},
            "visibility_signals": {"indexable": True},
        }])
        lead = db.get_lead(lead_id)
        self.assertEqual(lead["opportunity_gap_score"], 86)
        self.assertEqual(lead["visibility_reasons"]["seo"], ["weak"])
        rows = db.list_leads({"min_opportunity_gap_score": "80"})
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]["id"], lead_id)
        self.assertEqual(db.list_leads({"min_opportunity_gap_score": "90"}), [])

    def test_low_visibility_can_be_targeted_with_max_filters(self):
        db.upsert_leads([{
            "source": "test",
            "source_id": "gap-2",
            "name": "Strong Visibility Business",
            "country": "Pakistan",
            "city": "Karachi",
            "category": "dentist",
            "website": "https://strong.test",
            "website_status": "healthy",
            "seo_score": 88,
            "aeo_score": 82,
            "geo_score": 79,
            "ai_visibility_score": 83,
            "opportunity_gap_score": 22,
        }])
        rows = db.list_leads({"max_ai_visibility_score": "40"})
        self.assertEqual([row["source_id"] for row in rows], ["gap-1"])
        rows = db.list_leads({"max_seo_score": "35", "max_geo_score": "20"})
        self.assertEqual([row["source_id"] for row in rows], ["gap-1"])


if __name__ == "__main__":
    unittest.main()
