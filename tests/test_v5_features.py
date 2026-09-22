import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import lead_hunter.db as db
from lead_hunter.crm import add_activity, list_activities, set_engagement
from lead_hunter.intelligence import calculate_intelligence
from lead_hunter.merge import merge_leads
from lead_hunter.oauth_meta import oauth_start_url


class V5FeatureTests(unittest.TestCase):
    def setUp(self):
        self.original_db = db.DB_PATH
        self.tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self.tempdir.name) / "v5.db"
        db.initialize()

    def tearDown(self):
        db.DB_PATH = self.original_db
        self.tempdir.cleanup()

    def test_multi_source_merge(self):
        osm={
            "source":"osm","source_id":"node:1","name":"Acme Dental","category":"dentist",
            "latitude":41.0,"longitude":29.0,"phone":"+90 555 111 22 33",
            "website":None,"social_links":{},"website_status":"missing",
        }
        overture={
            "source":"overture","source_id":"ov-1","name":"Acme Dental","category":"dentist",
            "latitude":41.0001,"longitude":29.0001,"phone":"+905551112233",
            "website":"https://acme.example","email":"hello@acme.example",
            "social_links":{"instagram":"https://instagram.com/acme"},"website_status":"unknown",
        }
        rows=merge_leads([osm],[overture])
        self.assertEqual(len(rows),1)
        self.assertIn("osm",rows[0]["source_refs"])
        self.assertIn("overture",rows[0]["source_refs"])
        self.assertEqual(rows[0]["website"],"https://acme.example")

    def test_intelligence_scores_contactability(self):
        contact,commercial,reasons=calculate_intelligence({
            "category":"dentist","phone":"1","email":"x@example.com",
            "social_links":{"instagram":"x"},"website_status":"missing",
            "verification_status":"cross_source",
        })
        self.assertGreaterEqual(contact,70)
        self.assertGreaterEqual(commercial,50)
        self.assertTrue(reasons)

    def _lead(self):
        return db.upsert_leads([{
            "source":"test","source_id":"1","name":"Test Business","country":"TR","city":"Istanbul",
            "category":"dentist","website_status":"missing",
        }])[0]

    def test_crm_timeline_and_engagement(self):
        lead_id=self._lead()
        add_activity(lead_id,kind="note",body="called owner")
        set_engagement(lead_id,"rejected","not interested")
        rows=list_activities(lead_id)
        self.assertGreaterEqual(len(rows),2)
        self.assertEqual(db.get_lead(lead_id)["engagement_status"],"rejected")

    def test_meta_oauth_url_contains_state(self):
        with patch.dict(os.environ,{
            "META_APP_ID":"123",
            "META_APP_SECRET":"secret",
            "META_REDIRECT_URI":"http://127.0.0.1:8787/api/v1/oauth/meta/callback",
        },clear=False):
            url=oauth_start_url()
        self.assertIn("client_id=123",url)
        self.assertIn("state=",url)
        self.assertIn("redirect_uri=",url)


if __name__=="__main__":
    unittest.main()
