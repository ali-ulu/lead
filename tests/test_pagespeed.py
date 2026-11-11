import json
import unittest
from unittest.mock import patch

from lead_hunter.providers import pagespeed

SAMPLE = {
    "loadingExperience": {
        "overall_category": "AVERAGE",
        "metrics": {
            "LARGEST_CONTENTFUL_PAINT_MS": {"percentile": 2500, "category": "AVERAGE"},
            "CUMULATIVE_LAYOUT_SHIFT_SCORE": {"percentile": 5, "category": "FAST"},
        },
    },
    "lighthouseResult": {
        "lighthouseVersion": "12.0.0",
        "categories": {
            "performance": {"score": 0.72},
            "accessibility": {"score": 0.94},
            "best-practices": {"score": 1.0},
            "seo": {"score": 0.85},
        },
    },
}


class _FakeResponse:
    def __init__(self, body: bytes):
        self._body = body

    def read(self) -> bytes:
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


class PageSpeedParseTests(unittest.TestCase):
    def test_scores_are_scaled_to_percent(self):
        result = pagespeed.parse_response(SAMPLE)
        self.assertEqual(result["performance_score"], 72)
        self.assertEqual(result["accessibility_score"], 94)
        self.assertEqual(result["best_practices_score"], 100)
        self.assertEqual(result["seo_score"], 85)
        self.assertEqual(result["lighthouse_version"], "12.0.0")
        self.assertEqual(result["source"], "pagespeed")

    def test_field_data_is_extracted(self):
        result = pagespeed.parse_response(SAMPLE)
        field = result["field_data"]
        self.assertEqual(field["overall_category"], "AVERAGE")
        self.assertEqual(field["LARGEST_CONTENTFUL_PAINT_MS"]["percentile"], 2500)

    def test_missing_blocks_yield_none_scores(self):
        result = pagespeed.parse_response({})
        self.assertIsNone(result["performance_score"])
        self.assertIsNone(result["field_data"])


class PageSpeedConfigTests(unittest.TestCase):
    def setUp(self):
        import os
        self._saved = {k: os.environ.pop(k, None)
                       for k in ("GOOGLE_PSI_API_KEY", "LEADSCOUT_PAGESPEED")}

    def tearDown(self):
        import os
        for key, value in self._saved.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value

    def test_disabled_by_default(self):
        self.assertFalse(pagespeed.configured())

    def test_enabled_with_key(self):
        import os
        os.environ["GOOGLE_PSI_API_KEY"] = "k"
        self.assertTrue(pagespeed.configured())

    def test_enabled_with_flag(self):
        import os
        os.environ["LEADSCOUT_PAGESPEED"] = "1"
        self.assertTrue(pagespeed.configured())


class PageSpeedAnalyzeTests(unittest.TestCase):
    def setUp(self):
        self._patcher = patch.dict("os.environ", {"GOOGLE_PSI_API_KEY": "test-key"})
        self._patcher.start()

    def tearDown(self):
        self._patcher.stop()

    def test_analyze_parses_response(self):
        body = json.dumps(SAMPLE).encode()
        with patch.object(pagespeed.netguard, "build_public_opener") as opener:
            opener.return_value.open.return_value = _FakeResponse(body)
            result = pagespeed.analyze("https://example.com")
        self.assertEqual(result["performance_score"], 72)

    def test_api_error_raises(self):
        body = json.dumps({"error": {"message": "Quota exceeded"}}).encode()
        with patch.object(pagespeed.netguard, "build_public_opener") as opener:
            opener.return_value.open.return_value = _FakeResponse(body)
            with self.assertRaises(RuntimeError) as ctx:
                pagespeed.analyze("https://example.com")
        self.assertIn("Quota exceeded", str(ctx.exception))

    def test_disabled_analyze_raises(self):
        with patch.dict("os.environ", {"GOOGLE_PSI_API_KEY": "", "LEADSCOUT_PAGESPEED": ""}):
            with self.assertRaises(RuntimeError):
                pagespeed.analyze("https://example.com")


if __name__ == "__main__":
    unittest.main()
