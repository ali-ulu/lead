import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import lead_hunter.db as db
import lead_hunter.services as services
from lead_hunter.providers.web_search import _score_result


class V6VerificationTests(unittest.TestCase):
    def setUp(self):
        self.original_db_path=db.DB_PATH
        self.tempdir=tempfile.TemporaryDirectory()
        db.DB_PATH=Path(self.tempdir.name)/"leadscout-v6.db"
        db.initialize()
        [self.lead_id]=db.upsert_leads([{
            "source":"test",
            "source_id":"lead-1",
            "source_refs":{"test":"lead-1"},
            "name":"Example Dental",
            "country":"Pakistan",
            "city":"Karachi",
            "category":"dentist",
            "latitude":24.8607,
            "longitude":67.0011,
            "website":None,
            "website_status":"missing",
            "social_links":{},
            "data_confidence":"medium",
        }])

    def tearDown(self):
        db.DB_PATH=self.original_db_path
        self.tempdir.cleanup()

    def test_social_profile_is_not_treated_as_official_website(self):
        score=_score_result(
            {
                "url":"https://www.instagram.com/exampledental",
                "title":"Example Dental (@exampledental)",
                "description":"Karachi dentist",
            },
            name="Example Dental",
            city="Karachi",
            country="Pakistan",
        )
        self.assertEqual(score,0.0)

    @patch("lead_hunter.services.search_overture_bbox", return_value=[])
    @patch("lead_hunter.services.verify_business_web")
    def test_verify_lead_can_promote_web_search_site(self, web_verify, _overture):
        web_verify.return_value={
            "configured":True,
            "provider":"brave",
            "verified":True,
            "website":"https://exampledental.pk/",
            "confidence":0.91,
            "candidates":[],
        }
        result=services.verify_lead(self.lead_id)
        lead=result["lead"]
        self.assertEqual(lead["website"],"https://exampledental.pk/")
        self.assertEqual(lead["verification_status"],"web_verified")
        self.assertEqual(lead["website_status"],"unknown")

    @patch("lead_hunter.services.google_reputation")
    def test_reputation_updates_rating_review_count_and_identity(self, google):
        google.return_value={
            "configured":True,
            "matched":True,
            "match_confidence":0.94,
            "place_id":"places/example",
            "rating":4.7,
            "review_count":328,
            "website":"https://exampledental.pk/",
            "phone":"+92 21 0000000",
            "source":"google_places",
        }
        result=services.enrich_reputation(self.lead_id)
        lead=result["lead"]
        self.assertEqual(lead["rating"],4.7)
        self.assertEqual(lead["review_count"],328)
        self.assertEqual(lead["source_refs"]["google_places"],"places/example")
        self.assertEqual(lead["website"],"https://exampledental.pk/")
        self.assertGreater(lead["commercial_score"],0)


if __name__=="__main__":
    unittest.main()
