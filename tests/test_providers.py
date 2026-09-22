import unittest
from lead_hunter.providers.osm import _build_around_query, _social, _socials
from lead_hunter.audit import normalize_url, _assert_public_host, SignalParser
from lead_hunter.outreach import build_message

class ProviderTests(unittest.TestCase):
    def test_around_query_is_bounded(self):
        q = _build_around_query(24.8607, 67.0011, 20000, "dentist", 35)
        self.assertIn('amenity', q)
        self.assertIn('around:20000,24.8607,67.0011', q)
        self.assertIn('out center tags 250', q)

    def test_social_handle_becomes_url(self):
        self.assertEqual(
            _social({'contact:instagram':'@example'}),
            'https://instagram.com/example'
        )

    def test_multiple_social_channels(self):
        socials = _socials({
            'contact:instagram': '@nishan',
            'contact:facebook': 'nishan.pk',
            'contact:whatsapp': '+92 300 1234567',
        })
        self.assertEqual(socials['instagram'], 'https://instagram.com/nishan')
        self.assertEqual(socials['facebook'], 'https://facebook.com/nishan.pk')
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
