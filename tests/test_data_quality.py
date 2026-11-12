import unittest

from lead_hunter.merge import merge_leads
from lead_hunter.providers.nominatim import _pick_place
from lead_hunter.services import _within_radius


class GeocodePickTests(unittest.TestCase):
    def test_province_is_skipped_in_favour_of_city(self):
        rows = [
            {"display_name": "Afyonkarahisar, Turkey", "addresstype": "province",
             "lat": "38.68", "lon": "30.64", "boundingbox": ["37.7", "39.2", "29.6", "31.7"], "address": {}},
            {"display_name": "Afyonkarahisar Merkez, Turkey", "addresstype": "city",
             "lat": "38.75", "lon": "30.53", "boundingbox": ["38.5", "38.9", "30.1", "30.8"], "address": {}},
        ]
        picked = _pick_place(rows)
        self.assertEqual(picked["addresstype"], "city")

    def test_falls_back_to_first_row_when_no_settlement(self):
        rows = [{"display_name": "Nowhere", "addresstype": "province", "lat": "1", "lon": "2"}]
        self.assertIs(_pick_place(rows), rows[0])

    def test_prefers_city_over_suburb(self):
        rows = [
            {"addresstype": "suburb", "lat": "1", "lon": "1"},
            {"addresstype": "city", "lat": "2", "lon": "2"},
        ]
        self.assertEqual(_pick_place(rows)["lat"], "2")


class RadiusFilterTests(unittest.TestCase):
    # Afyonkarahisar centre; 1 degree latitude is ~111 km.
    CENTER = (38.6853, 30.6427)

    def _lead(self, lat, lon):
        return {"name": "x", "latitude": lat, "longitude": lon}

    def test_keeps_point_inside_circle(self):
        rows = [self._lead(38.69, 30.64)]  # ~0.5 km away
        self.assertEqual(len(_within_radius(rows, *self.CENTER, 5)), 1)

    def test_drops_bbox_corner_outside_circle(self):
        # ~7 km east: inside the 5 km bounding box corner but outside the circle.
        rows = [self._lead(38.6853, 30.7273)]
        self.assertEqual(_within_radius(rows, *self.CENTER, 5), [])

    def test_keeps_rows_without_coordinates(self):
        rows = [{"name": "no coords"}]
        self.assertEqual(len(_within_radius(rows, *self.CENTER, 5)), 1)


class BboxOverFetchTests(unittest.TestCase):
    def test_bbox_limit_compensates_for_circle_ratio(self):
        # A bounding box holds 4/pi more area than its inscribed circle, so the
        # provider limit must be scaled up or in-radius rows get crowded out.
        import math
        from lead_hunter.services import _bbox_limit

        self.assertEqual(_bbox_limit(None), None)
        self.assertEqual(_bbox_limit(300), math.ceil(300 * 4 / math.pi))
        self.assertGreater(_bbox_limit(300), 300)


class CrossSourceTests(unittest.TestCase):
    def _lead(self, source, source_id, **extra):
        base = {
            "source": source, "source_id": source_id, "name": "Acme Dental",
            "category": "dentist", "latitude": 41.0, "longitude": 29.0,
            "social_links": {}, "website_status": "missing",
        }
        base.update(extra)
        return base

    def test_single_source_claiming_cross_source_is_downgraded(self):
        rows = merge_leads([self._lead("overture", "ov-1", verification_status="cross_source")])
        self.assertEqual(rows[0]["verification_status"], "unverified")

    def test_two_matching_sources_stay_cross_source(self):
        osm = [self._lead("osm", "node:1", phone="0555 111 22 33")]
        overture = [self._lead("overture", "ov-1", phone="+905551112233")]
        rows = merge_leads(osm, overture)
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]["verification_status"], "cross_source")
        self.assertEqual(len(rows[0]["source_refs"]), 2)


if __name__ == "__main__":
    unittest.main()
