import unittest

from lead_hunter.merge import merge_leads


class MergeLeadsTests(unittest.TestCase):
    def test_dedupes_on_normalized_phone_formats(self):
        osm = [{
            "source": "osm", "source_id": "node:1", "name": "Acme Dental",
            "category": "dentist", "latitude": 41.0, "longitude": 29.0,
            "phone": "0555 111 22 33", "website": None, "social_links": {},
            "website_status": "missing",
        }]
        overture = [{
            "source": "overture", "source_id": "ov-1", "name": "Acme Dental",
            "category": "dentist", "latitude": 41.0001, "longitude": 29.0001,
            "phone": "+905551112233", "website": "https://acme.example",
            "social_links": {}, "website_status": "unknown",
        }]
        rows = merge_leads(osm, overture)
        self.assertEqual(len(rows), 1)
        self.assertIn("osm", rows[0]["source_refs"])
        self.assertIn("overture", rows[0]["source_refs"])

    def test_phone_is_stored_in_e164(self):
        rows = merge_leads([{
            "source": "osm", "source_id": "n:1", "name": "Kuafor A",
            "category": "barber", "latitude": 1.0, "longitude": 2.0,
            "phone": "0555 999 88 77", "social_links": {},
        }])
        self.assertEqual(rows[0]["phone"], "+905559998877")

    def test_website_is_stored_normalized(self):
        rows = merge_leads([{
            "source": "osm", "source_id": "n:2", "name": "Bella",
            "category": "restaurant", "latitude": 1.0, "longitude": 2.0,
            "website": "http://www.bella.example/?utm_source=x", "social_links": {},
            "website_status": "missing",
        }])
        self.assertEqual(rows[0]["website"], "https://bella.example")
        # A website present after normalization must not stay "missing".
        self.assertEqual(rows[0]["website_status"], "unknown")

    def test_fuzzy_name_merge_requires_proximity(self):
        near = [
            {"source": "osm", "source_id": "a", "name": "Sarıtaş Döner", "category": "restaurant",
             "latitude": 38.0, "longitude": 30.0, "social_links": {}},
            {"source": "overture", "source_id": "b", "name": "Sarıtaş Döner Salonu", "category": "restaurant",
             "latitude": 38.0001, "longitude": 30.0001, "social_links": {}},
        ]
        self.assertEqual(len(merge_leads(near)), 1)

        far = [
            {"source": "osm", "source_id": "a", "name": "Sarıtaş Döner", "category": "restaurant",
             "latitude": 38.0, "longitude": 30.0, "social_links": {}},
            {"source": "overture", "source_id": "b", "name": "Sarıtaş Döner Salonu", "category": "restaurant",
             "latitude": 39.5, "longitude": 32.0, "social_links": {}},
        ]
        self.assertEqual(len(merge_leads(far)), 2)

    def test_different_businesses_stay_separate(self):
        rows = merge_leads([
            {"source": "osm", "source_id": "a", "name": "Acme Dental", "category": "dentist",
             "latitude": 41.0, "longitude": 29.0, "social_links": {}},
            {"source": "osm", "source_id": "b", "name": "Bella Pizza", "category": "restaurant",
             "latitude": 41.0, "longitude": 29.0, "social_links": {}},
        ])
        self.assertEqual(len(rows), 2)

    def test_category_is_canonicalized(self):
        rows = merge_leads([
            {"source": "osm", "source_id": "a", "name": "Berber Ali", "category": "hairdresser",
             "latitude": 1.0, "longitude": 2.0, "social_links": {}},
        ])
        self.assertEqual(rows[0]["category"], "barber")

    def test_website_match_merges_across_distance(self):
        rows = merge_leads(
            [{"source": "osm", "source_id": "a", "name": "Alpha", "category": "hotel",
              "latitude": 1.0, "longitude": 2.0, "website": "https://www.alpha.example",
              "social_links": {}}],
            [{"source": "overture", "source_id": "b", "name": "Alpha Hotel", "category": "hotel",
              "latitude": 5.0, "longitude": 6.0, "website": "http://alpha.example/",
              "social_links": {}}],
        )
        self.assertEqual(len(rows), 1)


if __name__ == "__main__":
    unittest.main()
