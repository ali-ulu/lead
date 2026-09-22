import unittest
from unittest.mock import patch
from lead_hunter.providers.osm import _build_around_query, _fetch_tile_rows, _normalize, _social, _socials
from lead_hunter.audit import normalize_url, _assert_public_host, SignalParser
from lead_hunter.outreach import build_message


class ProviderTests(unittest.TestCase):
    def test_around_query_has_no_output_limit(self):
        q = _build_around_query(24.8607, 67.0011, 20000, "dentist", 35)
        self.assertIn('amenity', q)
        self.assertIn('around:20000,24.8607,67.0011', q)
        self.assertIn('out tags center qt;', q)
        self.assertNotIn(' 250;', q)

    def test_normalize_unlimited_mode(self):
        elements = [
            {"type": "node", "id": i, "lat": 1.0, "lon": 2.0, "tags": {"name": f"Business {i}"}}
            for i in range(600)
        ]
        rows = _normalize({"elements": elements}, "restaurant", "Test City", "Test Country", None)
        self.assertEqual(len(rows), 600)


    def test_timed_out_tile_returns_empty_instead_of_killing_search(self):
        with patch(
            "lead_hunter.providers.osm._fetch_one",
            side_effect=RuntimeError("timeout"),
        ):
            rows = _fetch_tile_rows(
                0.0, 0.0, 1.0, 1.0,
                "dentist", "Test City", "Test Country",
                timeout=1,
            )

        self.assertEqual(rows, [])

    def test_social_handle_becomes_url(self):
        self.assertEqual(
            _social({'contact:instagram':'@example'}),
            'https://instagram.com/example'
        )

    def test_multiple_social_channels(self):
        socials = _socials({
            'contact:instagram': '@leadscout',
            'contact:facebook': 'leadscout.example',
            'contact:whatsapp': '+92 300 1234567',
        })
        self.assertEqual(socials['instagram'], 'https://instagram.com/leadscout')
        self.assertEqual(socials['facebook'], 'https://facebook.com/leadscout.example')
        self.assertEqual(socials['whatsapp'], 'https://wa.me/923001234567')

    def test_website_parser_collects_social_links(self):
        parser = SignalParser()
        parser.feed(
            '<a href="https://www.instagram.com/example">Instagram</a>'
            '<a href="https://www.facebook.com/example">Facebook</a>'
        )
        self.assertIn('instagram', parser.social_links)
        self.assertIn('facebook', parser.social_links)

    def test_normalize_url(self):
        self.assertEqual(normalize_url('example.com'), 'https://example.com')

    def test_private_audit_target_is_blocked(self):
        with self.assertRaises(ValueError):
            _assert_public_host('http://localhost:8000')

    def test_urdu_outreach(self):
        message = build_message({'name':'Example','website':None,'website_status':'missing'}, 'ur')
        self.assertIn('السلام علیکم', message)

    def test_sindhi_outreach(self):
        message = build_message({'name':'Example','website':None,'website_status':'missing'}, 'sd')
        self.assertIn('السلام عليڪم', message)


if __name__ == '__main__':
    unittest.main()
