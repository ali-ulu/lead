"""Tests for ad/analytics tracking detection in the audit."""
import unittest

from lead_hunter.audit import SignalParser, detect_tracking


class DetectTrackingTests(unittest.TestCase):
    def test_detects_inline_gtag_and_tag_manager(self):
        vendors = detect_tracking(
            ["https://www.googletagmanager.com/gtag/js?id=G-XXXX"],
            ["window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}"],
        )
        self.assertIn("google_analytics", vendors)

    def test_detects_meta_pixel(self):
        vendors = detect_tracking([], ["!function(f,b,e,v,n,t,s){fbq('init','123')}(window);"])
        self.assertIn("meta_pixel", vendors)

    def test_multiple_vendors_sorted(self):
        vendors = detect_tracking(
            ["https://connect.facebook.net/en_US/fbevents.js", "https://mc.yandex.ru/metrika/tag.js"],
            ["fbq('track','PageView');"],
        )
        self.assertEqual(vendors, sorted(vendors))
        self.assertIn("meta_pixel", vendors)
        self.assertIn("yandex_metrica", vendors)

    def test_clean_page_has_no_vendors(self):
        self.assertEqual(detect_tracking(["/assets/app.js"], ["console.log('hi')"]), [])

    def test_no_scripts_no_false_positive(self):
        self.assertEqual(detect_tracking([], []), [])


class ParserCapturesScriptsTests(unittest.TestCase):
    def test_src_and_inline_bodies_are_collected(self):
        parser = SignalParser()
        parser.feed(
            "<html><head>"
            "<script src='https://example.com/app.js'></script>"
            "<script>fbq('init','1');</script>"
            "<script type='application/ld+json'>{\"@type\":\"LocalBusiness\"}</script>"
            "</head></html>"
        )
        self.assertIn("https://example.com/app.js", parser.script_srcs)
        self.assertTrue(any("fbq(" in body for body in parser.inline_scripts))
        # JSON-LD must not leak into the inline-script bucket.
        self.assertFalse(any("LocalBusiness" in body for body in parser.inline_scripts))
        self.assertEqual(len(parser.jsonld_blocks), 1)
        # Inline JS must not be counted as page text.
        self.assertNotIn("fbq('init','1');", " ".join(parser.text_parts))


if __name__ == "__main__":
    unittest.main()
