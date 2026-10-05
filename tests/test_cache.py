import os
import tempfile
import time
import unittest
from pathlib import Path
from unittest.mock import patch

from lead_hunter import cache, ratelimit


class CacheTests(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self._saved_dir = os.environ.get("LEADSCOUT_CACHE_DIR")
        self._saved_ttl = os.environ.get("LEADSCOUT_CACHE_TTL")
        os.environ["LEADSCOUT_CACHE_DIR"] = self.tempdir.name
        os.environ["LEADSCOUT_CACHE_TTL"] = "3600"

    def tearDown(self):
        for key, value in (("LEADSCOUT_CACHE_DIR", self._saved_dir), ("LEADSCOUT_CACHE_TTL", self._saved_ttl)):
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        self.tempdir.cleanup()

    def test_disabled_by_default(self):
        os.environ.pop("LEADSCOUT_CACHE_TTL", None)
        self.assertFalse(cache.enabled())
        cache.put("k", ["a"], {"v": 1})
        self.assertIsNone(cache.get("k", ["a"]))

    def test_round_trip_and_key_isolation(self):
        cache.put("overpass", ["q1"], {"elements": [1]})
        self.assertEqual(cache.get("overpass", ["q1"]), {"elements": [1]})
        self.assertIsNone(cache.get("overpass", ["q2"]))
        self.assertIsNone(cache.get("nominatim", ["q1"]))

    def test_expired_entry_is_ignored(self):
        cache.put("k", ["a"], {"v": 1})
        path = next(Path(self.tempdir.name).glob("*.json"))
        old = time.time() - 10_000
        os.utime(path, (old, old))
        self.assertIsNone(cache.get("k", ["a"]))

    def test_get_or_call_produces_once(self):
        calls = []

        def produce():
            calls.append(1)
            return {"n": len(calls)}

        self.assertEqual(cache.get_or_call("produce", ["a"], produce), {"n": 1})
        self.assertEqual(cache.get_or_call("produce", ["a"], produce), {"n": 1})
        self.assertEqual(len(calls), 1)


class RateLimitTests(unittest.TestCase):
    def setUp(self):
        ratelimit.reset()

    def tearDown(self):
        ratelimit.reset()

    def test_returns_value_and_retries_transient_errors(self):
        attempts = []

        def flaky():
            attempts.append(1)
            if len(attempts) < 3:
                raise OSError("temporary")
            return "ok"

        self.assertEqual(ratelimit.call("p", flaky, retries=3, backoff=0.0), "ok")
        self.assertEqual(len(attempts), 3)

    def test_non_transient_error_is_not_retried(self):
        attempts = []

        def broken():
            attempts.append(1)
            raise RuntimeError("bot-check")

        with self.assertRaises(RuntimeError):
            ratelimit.call("p", broken, retries=3, backoff=0.0)
        self.assertEqual(len(attempts), 1)

    def test_circuit_opens_after_repeated_failures(self):
        ratelimit.configure("p", failure_threshold=2, cooldown=60)

        def broken():
            raise RuntimeError("down")

        for _ in range(2):
            with self.assertRaises(RuntimeError):
                ratelimit.call("p", broken, retries=0)
        with self.assertRaises(ratelimit.CircuitOpen):
            ratelimit.call("p", lambda: "ok", retries=0)

    def test_success_resets_circuit(self):
        ratelimit.configure("p", failure_threshold=2, cooldown=60)

        def broken():
            raise RuntimeError("down")

        with self.assertRaises(RuntimeError):
            ratelimit.call("p", broken, retries=0)
        self.assertEqual(ratelimit.call("p", lambda: "ok", retries=0), "ok")
        with self.assertRaises(RuntimeError):
            ratelimit.call("p", broken, retries=0)
        # Only one failure since the reset, so the breaker is still closed.
        self.assertEqual(ratelimit.call("p", lambda: "ok", retries=0), "ok")

    def test_min_interval_spaces_calls(self):
        start = time.monotonic()
        ratelimit.call("p", lambda: 1, min_interval=0.05, retries=0)
        ratelimit.call("p", lambda: 2, min_interval=0.05, retries=0)
        self.assertGreaterEqual(time.monotonic() - start, 0.05)


class ProviderCacheIntegrationTests(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self._saved = {k: os.environ.get(k) for k in ("LEADSCOUT_CACHE_DIR", "LEADSCOUT_CACHE_TTL")}
        os.environ["LEADSCOUT_CACHE_DIR"] = self.tempdir.name
        os.environ["LEADSCOUT_CACHE_TTL"] = "3600"
        ratelimit.reset()

    def tearDown(self):
        for key, value in self._saved.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        ratelimit.reset()
        self.tempdir.cleanup()

    def test_nominatim_second_call_hits_cache(self):
        from lead_hunter.providers import nominatim
        payload = b'[{"display_name":"Test","lat":"1.0","lon":"2.0","boundingbox":["0.9","1.1","1.9","2.1"],"address":{"country":"Testland","country_code":"tl","city":"Test"}}]'

        class Resp:
            def read(self):
                return payload

            def __enter__(self):
                return self

            def __exit__(self, *exc):
                return False

        with patch.object(nominatim.urllib.request, "urlopen", return_value=Resp()) as mock:
            first = nominatim.geocode_area("Test", "Testland")
            second = nominatim.geocode_area("Test", "Testland")
        self.assertEqual(first["lat"], 1.0)
        self.assertEqual(second["lat"], 1.0)
        self.assertEqual(mock.call_count, 1)

    def test_overpass_tile_scan_runs_tiles_in_parallel(self):
        from lead_hunter.providers import osm
        seen = []

        def fake_fetch_one(endpoint, query, timeout):
            seen.append(query)
            return {"elements": []}

        with patch.object(osm, "_fetch_one", side_effect=fake_fetch_one):
            result = osm._search_tiled_around_detailed(1.0, 2.0, 10, "restaurant", "Test", "Testland", 35, None)
        self.assertEqual(len(seen), 4)
        self.assertEqual(result["rows"], [])


if __name__ == "__main__":
    unittest.main()
