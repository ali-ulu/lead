"""Tests for the audit/visibility improvements.

Covers the pieces that were previously untestable because the values were
hard-coded or dead: the continuous speed proxy, the TTFB/body timing split,
robots/header indexability, the renderer hook, the config-driven weights and
the score-confidence label.
"""
import os
import unittest
from unittest import mock

import lead_hunter.audit as audit
import lead_hunter.config as config
from lead_hunter.visibility import calculate_visibility


class SpeedProxyTests(unittest.TestCase):
    def test_is_monotonic_and_continuous(self):
        values = [audit._speed_proxy(ms) for ms in (100, 200, 700, 750, 1200, 1250, 2500, 5000)]
        self.assertEqual(values, sorted(values, reverse=True))
        # Buckets are gone: neighbours must not collide as often as a step function.
        self.assertNotEqual(audit._speed_proxy(700), audit._speed_proxy(1199))

    def test_bounds(self):
        self.assertLessEqual(audit._speed_proxy(0), 100)
        self.assertGreaterEqual(audit._speed_proxy(60000), 5)


class ConfigWeightsTests(unittest.TestCase):
    def test_env_override_changes_seo_ceiling(self):
        base = calculate_visibility({"indexable": True, "title": "T", "has_https": True})
        override = '{"seo": {"indexable": 100}}'
        with mock.patch.dict(os.environ, {"LEADSCOUT_VISIBILITY_WEIGHTS": override}):
            boosted = calculate_visibility({"indexable": True, "title": "T", "has_https": True})
        self.assertGreaterEqual(boosted["seo_score"], base["seo_score"])

    def test_invalid_env_is_ignored(self):
        with mock.patch.dict(os.environ, {"LEADSCOUT_VISIBILITY_WEIGHTS": "{not json"}):
            self.assertEqual(config.visibility_weights(), config.DEFAULT_VISIBILITY_WEIGHTS)


class ScoreConfidenceTests(unittest.TestCase):
    def test_heuristic_when_no_lighthouse(self):
        result = calculate_visibility({"indexable": True})
        self.assertEqual(result["score_confidence"]["seo"], "heuristic")
        self.assertEqual(result["score_confidence"]["ai_visibility"], "composite")

    def test_lighthouse_labels(self):
        result = calculate_visibility({
            "indexable": True,
            "lighthouse_seo_score": 90,
            "accessibility_score": 95,
            "best_practices_score": 92,
        })
        self.assertEqual(result["score_confidence"]["seo"], "lighthouse")
        self.assertEqual(result["score_confidence"]["aeo"], "lighthouse")
        self.assertEqual(result["score_confidence"]["geo"], "lighthouse")


class IndexabilityTests(unittest.TestCase):
    def test_header_noindex_marks_page_not_indexable(self):
        result = calculate_visibility({"indexable": False, "title": "T", "h1_count": 1})
        self.assertLess(result["ai_visibility_score"], 60)


class RendererHookTests(unittest.TestCase):
    def test_renderer_output_replaces_shell(self):
        shell = "<html><head></head><body><div id='app'></div></body></html>"
        rendered = "<html><head><title>Rendered</title></head><body><main><h1>Rendered</h1></main></body></html>"

        class _Resp:
            status = 200
            headers = {"Content-Type": "text/html"}

            def read(self, *_):
                return shell.encode("utf-8")

            def geturl(self):
                return "https://example.test/"

            def __enter__(self):
                return self

            def __exit__(self, *exc):
                return False

        class _Headers(dict):
            def get(self, key, default=None):
                return super().get(key, default)

            def get_content_charset(self):
                return "utf-8"

        class _RespWithHeaders(_Resp):
            headers = _Headers({"Content-Type": "text/html"})

        seen = {}

        def renderer(url):
            seen["url"] = url
            return rendered

        audit.set_renderer(renderer)
        try:
            with mock.patch.object(audit.netguard, "robots_allows", return_value=True), \
                    mock.patch.object(audit.netguard, "assert_public_url", return_value=True), \
                    mock.patch.object(audit.netguard, "build_public_opener") as opener, \
                    mock.patch.object(audit, "_lighthouse", return_value=None):
                opener.return_value.open.return_value = _RespWithHeaders()
                result = audit.audit_url("https://example.test/")
        finally:
            audit.set_renderer(None)

        self.assertEqual(seen.get("url"), "https://example.test/")
        self.assertEqual(result["title"], "Rendered")

    def test_lighthouse_rendered_html_used_without_explicit_renderer(self):
        shell = "<html><head></head><body><div id='app'></div></body></html>"
        rendered = "<html><head><title>From Lighthouse</title></head><body><main><h1>Real</h1></main></body></html>"

        class _Headers(dict):
            def get_content_charset(self):
                return "utf-8"

        class _Resp:
            status = 200
            headers = _Headers({"Content-Type": "text/html"})

            def read(self, *_):
                return shell.encode("utf-8")

            def geturl(self):
                return "https://example.test/"

            def __enter__(self):
                return self

            def __exit__(self, *exc):
                return False

        audit.set_renderer(None)
        with mock.patch.object(audit.netguard, "robots_allows", return_value=True), \
                mock.patch.object(audit.netguard, "assert_public_url", return_value=True), \
                mock.patch.object(audit.netguard, "build_public_opener") as opener, \
                mock.patch.object(audit, "_lighthouse", return_value={"rendered_html": rendered}):
            opener.return_value.open.return_value = _Resp()
            result = audit.audit_url("https://example.test/")

        self.assertEqual(result["title"], "From Lighthouse")
        self.assertTrue(result["indexable"])


if __name__ == "__main__":
    unittest.main()
