import hashlib
import hmac
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import lead_hunter.db as db
from lead_hunter.crm import (
    add_activity,
    list_activities,
    set_engagement,
    update_delivery_status,
)
from lead_hunter.intelligence import calculate_intelligence
from lead_hunter.merge import merge_leads
from lead_hunter.oauth_meta import (
    facebook_oauth_start_url,
    instagram_oauth_start_url,
    messaging_eligibility,
    verify_webhook_signature,
)


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

    def test_delivery_receipt_updates_message_and_lead(self):
        lead_id=self._lead()
        add_activity(
            lead_id,
            kind="message",
            channel="facebook",
            status="sent",
            direction="out",
            body="hello",
            external_id="mid.123",
        )
        self.assertTrue(update_delivery_status("mid.123","delivered"))
        rows=list_activities(lead_id)
        message=next(x for x in rows if x.get("external_id")=="mid.123")
        self.assertEqual(message["status"],"delivered")
        self.assertEqual(db.get_lead(lead_id)["engagement_status"],"delivered")

    def test_facebook_oauth_url_contains_state_and_messaging_scopes(self):
        with patch.dict(os.environ,{
            "META_APP_ID":"123",
            "META_APP_SECRET":"secret",
            "META_FACEBOOK_REDIRECT_URI":"http://127.0.0.1:8787/api/v1/oauth/meta/facebook/callback",
        },clear=False):
            url=facebook_oauth_start_url()
        self.assertIn("client_id=123",url)
        self.assertIn("state=",url)
        self.assertIn("pages_messaging",url)

    def test_instagram_business_login_uses_current_business_scopes(self):
        with patch.dict(os.environ,{
            "META_INSTAGRAM_APP_ID":"456",
            "META_INSTAGRAM_APP_SECRET":"secret",
            "META_INSTAGRAM_REDIRECT_URI":"http://127.0.0.1:8787/api/v1/oauth/meta/instagram/callback",
        },clear=False):
            url=instagram_oauth_start_url()
        self.assertTrue(url.startswith("https://www.instagram.com/oauth/authorize?"))
        self.assertIn("instagram_business_basic",url)
        self.assertIn("instagram_business_manage_messages",url)
        self.assertIn("state=",url)

    def test_webhook_signature_validation(self):
        raw=b'{"object":"page","entry":[]}'
        secret="top-secret"
        sig="sha256="+hmac.new(secret.encode(),raw,hashlib.sha256).hexdigest()
        with patch.dict(os.environ,{"META_APP_SECRET":secret},clear=False):
            self.assertTrue(verify_webhook_signature(raw,sig,"facebook"))
            self.assertFalse(verify_webhook_signature(raw,"sha256=bad","facebook"))

    def test_messaging_eligibility_requires_real_recipient_id(self):
        lead_id=self._lead()
        first=messaging_eligibility(lead_id,"instagram")
        self.assertFalse(first["eligible"])
        db.update_lead(lead_id,{"messaging_ids":{"instagram":"IGSID-1"}})
        second=messaging_eligibility(lead_id,"instagram")
        self.assertTrue(second["eligible"])
        self.assertIn("already messaged",second["rule"])


if __name__=="__main__":
    unittest.main()
