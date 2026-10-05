import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import lead_hunter.db as db
import lead_hunter.services as services
from lead_hunter.providers.web_search import (
    _duckduckgo_search,
    _google_search,
    _score_result,
    configured_provider,
    configured_providers,
    verify_business_web,
)


class _FakeResponse:
    def __init__(self, body: bytes):
        self._body = body

    def read(self) -> bytes:
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


DDG_HTML = b"""
<div class="result">
  <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample-dental.pk%2F">Example Dental</a>
  <a class="result__snippet">Example Dental in Karachi, official website</a>
</div>
<div class="result">
  <a class="result__a" href="https://instagram.com/exampledental">Example Dental (@exampledental)</a>
</div>
"""

GOOGLE_JSON = b"""
{"items": [
  {"link": "https://example-dental.pk/", "title": "Example Dental", "snippet": "Karachi dentist official website"},
  {"link": "https://yelp.com/biz/example-dental", "title": "Example Dental - Yelp", "snippet": "Reviews"}
]}
"""


class WebSearchProviderTests(unittest.TestCase):
    _KEYS = ("LEADSCOUT_WEB_SEARCH", "BRAVE_SEARCH_API_KEY", "SEARXNG_URL",
             "GOOGLE_CSE_API_KEY", "GOOGLE_CSE_ID")

    def setUp(self):
        import os
        self._saved = {key: os.environ.pop(key, None) for key in self._KEYS}

    def tearDown(self):
        import os
        for key, value in self._saved.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value

    def test_default_chain_is_keyless_then_google(self):
        import os
        self.assertEqual(configured_providers(), ["duckduckgo"])
        self.assertEqual(configured_provider(), "duckduckgo")
        os.environ["GOOGLE_CSE_API_KEY"] = "key"
        os.environ["GOOGLE_CSE_ID"] = "cx"
        self.assertEqual(configured_providers(), ["duckduckgo", "google"])

    def test_google_needs_both_key_and_cx(self):
        import os
        os.environ["GOOGLE_CSE_API_KEY"] = "key"
        self.assertNotIn("google", configured_providers())

    def test_explicit_chain_and_off(self):
        import os
        os.environ["GOOGLE_CSE_API_KEY"] = "key"
        os.environ["GOOGLE_CSE_ID"] = "cx"
        os.environ["SEARXNG_URL"] = "http://127.0.0.1:8080"
        os.environ["LEADSCOUT_WEB_SEARCH"] = "searxng, google , duckduckgo"
        self.assertEqual(configured_providers(), ["searxng", "google", "duckduckgo"])
        self.assertEqual(configured_provider(), "searxng")
        os.environ["LEADSCOUT_WEB_SEARCH"] = "brave"
        self.assertEqual(configured_providers(), [])
        self.assertEqual(configured_provider(), "")
        os.environ["LEADSCOUT_WEB_SEARCH"] = "off"
        self.assertEqual(configured_providers(), [])

    def test_duckduckgo_results_are_parsed_and_unwrapped(self):
        with patch(
            "lead_hunter.providers.web_search.urllib.request.urlopen",
            return_value=_FakeResponse(DDG_HTML),
        ):
            results = _duckduckgo_search("Example Dental Karachi", count=5)
        self.assertEqual(results[0]["url"], "https://example-dental.pk/")
        self.assertEqual(results[0]["title"], "Example Dental")
        self.assertIn("official website", results[0]["description"])
        self.assertEqual(results[1]["url"], "https://instagram.com/exampledental")

    def test_google_results_are_parsed(self):
        import os
        os.environ["GOOGLE_CSE_API_KEY"] = "key"
        os.environ["GOOGLE_CSE_ID"] = "cx"
        with patch(
            "lead_hunter.providers.web_search.urllib.request.urlopen",
            return_value=_FakeResponse(GOOGLE_JSON),
        ):
            results = _google_search("Example Dental Karachi", count=5)
        self.assertEqual(results[0]["url"], "https://example-dental.pk/")
        self.assertEqual(results[0]["title"], "Example Dental")
        self.assertIn("official website", results[0]["description"])

    def test_duckduckgo_fallback_scores_as_official_site(self):
        with patch(
            "lead_hunter.providers.web_search.urllib.request.urlopen",
            return_value=_FakeResponse(DDG_HTML),
        ):
            check = verify_business_web(name="Example Dental", city="Karachi", country="Pakistan")
        self.assertTrue(check["configured"])
        self.assertEqual(check["provider"], "duckduckgo")
        self.assertTrue(check["verified"])
        self.assertEqual(check["website"], "https://example-dental.pk/")

    def test_falls_back_to_google_when_duckduckgo_fails(self):
        import os
        os.environ["GOOGLE_CSE_API_KEY"] = "key"
        os.environ["GOOGLE_CSE_ID"] = "cx"
        calls = []

        def fake_urlopen(req, timeout=20):
            calls.append(req.full_url)
            if "duckduckgo" in req.full_url:
                raise OSError("rate limited")
            return _FakeResponse(GOOGLE_JSON)

        with patch("lead_hunter.providers.web_search.urllib.request.urlopen", side_effect=fake_urlopen):
            check = verify_business_web(name="Example Dental", city="Karachi", country="Pakistan")
        self.assertEqual(check["provider"], "google")
        self.assertTrue(check["verified"])
        self.assertEqual(check["website"], "https://example-dental.pk/")
        self.assertEqual(check["errors"][0]["provider"], "duckduckgo")
        self.assertEqual(len(calls), 2)

    def test_duckduckgo_botcheck_page_raises(self):
        botcheck = b'<form id="img-form" action="//duckduckgo.com/anomaly.js?sv=html"></form>'
        with patch(
            "lead_hunter.providers.web_search.urllib.request.urlopen",
            return_value=_FakeResponse(botcheck),
        ):
            with self.assertRaises(RuntimeError):
                _duckduckgo_search("anything", count=5)

    def test_empty_results_fall_through_to_next_provider(self):
        import os
        os.environ["GOOGLE_CSE_API_KEY"] = "key"
        os.environ["GOOGLE_CSE_ID"] = "cx"
        empty = b"<html><body>no results here</body></html>"
        calls = []

        def fake_urlopen(req, timeout=20):
            calls.append(req.full_url)
            if "duckduckgo" in req.full_url:
                return _FakeResponse(empty)
            return _FakeResponse(GOOGLE_JSON)

        with patch("lead_hunter.providers.web_search.urllib.request.urlopen", side_effect=fake_urlopen):
            check = verify_business_web(name="Example Dental", city="Karachi", country="Pakistan")
        self.assertEqual(check["provider"], "google")
        self.assertTrue(check["verified"])
        self.assertEqual(len(calls), 2)


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
        self.assertEqual(lead["website"],"https://exampledental.pk")
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
        self.assertEqual(lead["website"],"https://exampledental.pk")
        self.assertGreater(lead["commercial_score"],0)


if __name__=="__main__":
    unittest.main()
