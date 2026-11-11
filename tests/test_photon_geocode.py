"""Tests for the Photon geocoding fallback."""
import unittest
from unittest.mock import patch

from lead_hunter.providers import photon


_CITY_PAYLOAD = (
    b'{"features":[{"properties":{"name":"Berlin","city":"Berlin",'
    b'"country":"Deutschland","countrycode":"DE",'
    b'"extent":[13.088345,52.6755087,13.7611609,52.3382448]},'
    b'"geometry":{"coordinates":[13.3951309,52.5173885]}}]}'
)

_POINT_PAYLOAD = (
    b'{"features":[{"properties":{"name":"Somewhere","city":"Somewhere",'
    b'"country":"Testland","countrycode":"tl"},'
    b'"geometry":{"coordinates":[2.0,1.0]}}]}'
)


class _Resp:
    def __init__(self, payload: bytes):
        self._payload = payload

    def read(self):
        return self._payload

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


class PhotonGeocodeTests(unittest.TestCase):
    def test_extent_becomes_bbox(self):
        with patch.object(photon.urllib.request, "urlopen", return_value=_Resp(_CITY_PAYLOAD)):
            area = photon.geocode_area("Berlin", "Germany")
        self.assertEqual(area["lat"], 52.5173885)
        self.assertEqual(area["lon"], 13.3951309)
        self.assertEqual(area["bbox"], {"south": 52.3382448, "north": 52.6755087, "west": 13.088345, "east": 13.7611609})
        self.assertEqual(area["country_code"], "DE")

    def test_point_result_derives_padded_bbox(self):
        with patch.object(photon.urllib.request, "urlopen", return_value=_Resp(_POINT_PAYLOAD)):
            area = photon.geocode_area("Somewhere", "Testland")
        self.assertLess(area["bbox"]["south"], area["lat"])
        self.assertGreater(area["bbox"]["north"], area["lat"])
        self.assertLess(area["bbox"]["west"], area["lon"])
        self.assertGreater(area["bbox"]["east"], area["lon"])

    def test_empty_features_raises_lookup(self):
        with patch.object(photon.urllib.request, "urlopen", return_value=_Resp(b'{"features":[]}')):
            with self.assertRaises(LookupError):
                photon.geocode_area("Nowhere", "")

    def test_missing_query_rejected(self):
        with self.assertRaises(ValueError):
            photon.geocode_area("", "")


class GeocodeFallbackTests(unittest.TestCase):
    def test_uses_photon_when_nominatim_fails(self):
        from lead_hunter import services

        area = {"display_name": "Fallback", "lat": 1.0, "lon": 2.0, "bbox": {}, "country": "TL", "country_code": "TL", "city": "Fallback"}
        with patch.object(services, "geocode_area", side_effect=RuntimeError("nominatim down")), \
                patch.object(services, "photon_geocode_area", return_value=area):
            resolved, warnings = services._geocode_area_with_fallback("Fallback", "TL")
        self.assertEqual(resolved["lat"], 1.0)
        self.assertTrue(any("Photon" in w for w in warnings))

    def test_prefers_nominatim_when_available(self):
        from lead_hunter import services

        area = {"display_name": "Primary", "lat": 3.0, "lon": 4.0, "bbox": {}, "country": "TL", "country_code": "TL", "city": "Primary"}
        with patch.object(services, "geocode_area", return_value=area), \
                patch.object(services, "photon_geocode_area", side_effect=AssertionError("should not be called")):
            resolved, warnings = services._geocode_area_with_fallback("Primary", "TL")
        self.assertEqual(resolved["lat"], 3.0)
        self.assertEqual(warnings, [])

    def test_both_failing_propagates_primary_error(self):
        from lead_hunter import services

        with patch.object(services, "geocode_area", side_effect=RuntimeError("nominatim down")), \
                patch.object(services, "photon_geocode_area", side_effect=RuntimeError("photon down")):
            with self.assertRaises(RuntimeError) as ctx:
                services._geocode_area_with_fallback("Nowhere", "")
        self.assertIn("nominatim down", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
