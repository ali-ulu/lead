import unittest
from lead_hunter.providers.osm import _build_around_query, _social
from lead_hunter.audit import normalize_url, _assert_public_host

class ProviderTests(unittest.TestCase):
    def test_around_query_is_bounded(self):
        q = _build_around_query(52.52, 13.405, 20000, "dentist", 35)
        self.assertIn('amenity', q)
        self.assertIn('around:20000,52.52,13.405', q)
        self.assertIn('out center tags 250', q)

    def test_social_handle_becomes_url(self):
        self.assertEqual(_social({'contact:instagram':'@example'}), 'https://instagram.com/example')

    def test_normalize_url(self):
        self.assertEqual(normalize_url('example.com'), 'https://example.com')

    def test_private_audit_target_is_blocked(self):
        with self.assertRaises(ValueError):
            _assert_public_host('http://localhost:8000')

if __name__ == '__main__':
    unittest.main()
