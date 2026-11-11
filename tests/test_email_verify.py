"""Tests for the email verification provider."""
import unittest
from unittest.mock import patch

from lead_hunter.providers import email_verify


class EmailVerifyTests(unittest.TestCase):
    def test_malformed_address_is_invalid(self):
        result = email_verify.verify_email("not-an-email", check_mx=False)
        self.assertFalse(result["valid_syntax"])
        self.assertEqual(result["status"], "invalid")
        self.assertEqual(result["score"], 0)

    def test_business_address_is_valid(self):
        result = email_verify.verify_email("owner@localplumber.co.uk", check_mx=False)
        self.assertTrue(result["valid_syntax"])
        self.assertEqual(result["status"], "valid")
        self.assertFalse(result["role_account"])

    def test_role_account_is_penalised(self):
        result = email_verify.verify_email("info@localplumber.co.uk", check_mx=False)
        self.assertTrue(result["role_account"])
        self.assertLess(result["score"], 100)

    def test_disposable_domain_is_invalid(self):
        result = email_verify.verify_email("someone@mailinator.com", check_mx=False)
        self.assertTrue(result["disposable"])
        self.assertEqual(result["status"], "invalid")

    def test_free_provider_is_flagged(self):
        result = email_verify.verify_email("someone@gmail.com", check_mx=False)
        self.assertTrue(result["free_provider"])
        self.assertEqual(result["status"], "valid")  # still deliverable

    def test_missing_mx_marks_invalid(self):
        with patch.object(email_verify, "_mx_lookup", return_value=False):
            result = email_verify.verify_email("owner@dead-domain.test", check_mx=True)
        self.assertFalse(result["mx_found"])
        self.assertEqual(result["status"], "invalid")

    def test_present_mx_stays_valid(self):
        with patch.object(email_verify, "_mx_lookup", return_value=True):
            result = email_verify.verify_email("owner@localplumber.co.uk", check_mx=True)
        self.assertTrue(result["mx_found"])
        self.assertEqual(result["status"], "valid")

    def test_lowercases_and_trims(self):
        result = email_verify.verify_email("  OWNER@Shop.COM ", check_mx=False)
        self.assertEqual(result["email"], "owner@shop.com")

    def test_asset_filename_is_not_an_address(self):
        # A retina image name such as "sprite@2x.png" matches the address regex
        # because the extension reads as a TLD; it must not be treated as one.
        result = email_verify.verify_email("find_provider@2x-480x358.png", check_mx=False)
        self.assertFalse(result["valid_syntax"])
        self.assertEqual(result["status"], "invalid")
        self.assertEqual(result["score"], 0)

    def test_looks_like_email_filters_assets_keeps_real(self):
        for good in ("owner@shop.com", "info@firma.com.tr", "a.b+c@mail.example.org"):
            self.assertTrue(email_verify.looks_like_email(good), good)
        for bad in ("sprite@2x.png", "icon@3x-480x358.jpg", "logo@2x.svg",
                    "bundle@app.js", "font@2x.woff2"):
            self.assertFalse(email_verify.looks_like_email(bad), bad)

    def test_crawl_drops_asset_names_and_keeps_real_address(self):
        from lead_hunter import enrichment
        html = (
            '<html><body>'
            '<img src="/img/find_provider@2x-480x358.png">'
            '<img src="/img/search_formulary@2x.png">'
            '<a href="mailto:owner@shop.com">mail</a>'
            '<p>Reach us at info@shop.com.tr</p>'
            '</body></html>'
        )
        with patch.object(enrichment, "_fetch", return_value=("https://shop.com/", html)):
            result = enrichment.enrich_website("https://shop.com/")
        self.assertIn("owner@shop.com", result["emails"])
        self.assertIn("info@shop.com.tr", result["emails"])
        self.assertFalse([e for e in result["emails"] if e.endswith(".png")])


if __name__ == "__main__":
    unittest.main()
