"""Security tests: SSRF guards, robots.txt, rate limiting, token gate, erasure."""
import socket
import time
import unittest
from unittest import mock

from lead_hunter import netguard


def _addr(ip: str, port: int = 80):
    return [(socket.AF_INET, socket.SOCK_STREAM, 6, "", (ip, port))]


class SsrfGuardTests(unittest.TestCase):
    def test_public_host_allowed(self):
        with mock.patch.object(netguard.socket, "getaddrinfo", return_value=_addr("93.184.216.34")):
            self.assertEqual(netguard.assert_public_url("https://example.com/"), "https://example.com/")

    def test_loopback_and_private_blocked(self):
        for ip in ("127.0.0.1", "10.0.0.5", "192.168.1.1", "172.16.0.1",
                   "169.254.169.254", "0.0.0.0"):
            with self.subTest(ip=ip):
                with mock.patch.object(netguard.socket, "getaddrinfo", return_value=_addr(ip)):
                    with self.assertRaises(ValueError) as ctx:
                        netguard.assert_public_url("http://internal.example/")
                self.assertIn("Local/private", str(ctx.exception))

    def test_localhost_name_blocked_without_dns(self):
        with self.assertRaises(ValueError):
            netguard.assert_public_url("http://localhost:8000/admin")

    def test_unresolvable_reported(self):
        with mock.patch.object(netguard.socket, "getaddrinfo",
                               side_effect=socket.gaierror("Name or service not known")):
            with self.assertRaises(ValueError) as ctx:
                netguard.assert_public_url("http://retired.example/")
        self.assertIn("could not be resolved", str(ctx.exception))

    def test_redirect_handler_rechecks_each_hop(self):
        handler = netguard._PublicRedirectHandler()
        req = mock.Mock(full_url="https://example.com/")
        with mock.patch.object(netguard.socket, "getaddrinfo", return_value=_addr("127.0.0.1")):
            with self.assertRaises(ValueError):
                handler.redirect_request(req, None, 302, "Found", {}, "http://internal/")

    def test_guard_connection_rejects_private_peer(self):
        conn = netguard._GuardHTTPConnection("example.com", 80, timeout=1)
        fake = mock.Mock()
        fake.getpeername.return_value = ("127.0.0.1", 80)
        with mock.patch.object(conn, "_create_connection", return_value=fake):
            with self.assertRaises(ValueError):
                conn.connect()
        fake.close.assert_called_once()

    def test_guard_connection_allows_public_peer(self):
        conn = netguard._GuardHTTPConnection("example.com", 80, timeout=1)
        fake = mock.Mock()
        fake.getpeername.return_value = ("93.184.216.34", 80)
        with mock.patch.object(conn, "_create_connection", return_value=fake):
            conn.connect()
        self.assertIs(conn.sock, fake)


class RobotsTests(unittest.TestCase):
    def test_parse_specific_agent_rules(self):
        body = "User-agent: *\nDisallow: /all\nUser-agent: LeadScout\nDisallow: /private\nDisallow: /tmp*\n"
        rules = netguard._parse_robots(body)
        self.assertEqual(sorted(rules), ["/private", "/tmp*"])

    def test_wildcard_group_used_when_no_specific(self):
        rules = netguard._parse_robots("User-agent: *\nDisallow: /secret\nAllow: /\n")
        self.assertEqual(rules, ["/secret"])

    def test_allows_and_blocks_paths(self):
        with mock.patch.object(netguard, "_fetch_robots", return_value=["/private", "/tmp*"]), \
                mock.patch.object(netguard, "_ROBOTS_CACHE", {}):
            self.assertFalse(netguard.robots_allows("https://example.com/private/page"))
            self.assertFalse(netguard.robots_allows("https://example.com/tmp/one"))
            self.assertTrue(netguard.robots_allows("https://example.com/public"))

    def test_root_disallow_blocks_all(self):
        with mock.patch.object(netguard, "_fetch_robots", return_value=["/"]), \
                mock.patch.object(netguard, "_ROBOTS_CACHE", {}):
            self.assertFalse(netguard.robots_allows("https://example.com/anything"))

    def test_unreachable_robots_allows(self):
        with mock.patch.object(netguard, "_fetch_robots", return_value=[]), \
                mock.patch.object(netguard, "_ROBOTS_CACHE", {}):
            self.assertTrue(netguard.robots_allows("https://example.com/"))


class RateLimiterTests(unittest.TestCase):
    def test_spaces_calls(self):
        limiter = netguard.RateLimiter(0.05)
        start = time.monotonic()
        limiter.wait()
        limiter.wait()
        self.assertGreaterEqual(time.monotonic() - start, 0.04)

    def test_disabled_is_noop(self):
        limiter = netguard.RateLimiter(0)
        start = time.monotonic()
        for _ in range(5):
            limiter.wait()
        self.assertLess(time.monotonic() - start, 0.02)


class ServerGateTests(unittest.TestCase):
    def _handler(self, headers, host="127.0.0.1"):
        import server
        obj = object.__new__(server.Handler)
        obj.headers = headers
        return server, obj

    def test_token_required_when_configured(self):
        server, obj = self._handler({})
        with mock.patch.object(server, "API_TOKEN", "secret"), \
                mock.patch.object(server, "HOST", "127.0.0.1"):
            self.assertFalse(server.Handler._authorized(obj, "/api/v1/leads"))
            obj.headers = {"Authorization": "Bearer secret"}
            self.assertTrue(server.Handler._authorized(obj, "/api/v1/leads"))

    def test_health_is_public(self):
        server, obj = self._handler({})
        with mock.patch.object(server, "API_TOKEN", "secret"):
            self.assertTrue(server.Handler._authorized(obj, "/api/v1/health"))

    def test_public_bind_without_token_closes_api(self):
        server, obj = self._handler({})
        with mock.patch.object(server, "API_TOKEN", ""), \
                mock.patch.object(server, "HOST", "0.0.0.0"):
            self.assertFalse(server.Handler._authorized(obj, "/api/v1/leads"))

    def test_loopback_without_token_allows(self):
        server, obj = self._handler({})
        with mock.patch.object(server, "API_TOKEN", ""), \
                mock.patch.object(server, "HOST", "127.0.0.1"):
            self.assertTrue(server.Handler._authorized(obj, "/api/v1/leads"))

    def test_static_is_always_public(self):
        server, obj = self._handler({})
        with mock.patch.object(server, "API_TOKEN", "secret"):
            self.assertTrue(server.Handler._authorized(obj, "/index.html"))

    def test_legacy_api_closed_on_public_bind(self):
        server, obj = self._handler({})
        with mock.patch.object(server, "API_TOKEN", ""), \
                mock.patch.object(server, "HOST", "0.0.0.0"):
            self.assertFalse(server.Handler._authorized(obj, "/api/leads"))
            self.assertFalse(server.Handler._authorized(obj, "/api/clear"))
            # health stays public so a load balancer can probe it
            self.assertTrue(server.Handler._authorized(obj, "/api/health"))

    def test_legacy_api_requires_token_when_configured(self):
        server, obj = self._handler({})
        with mock.patch.object(server, "API_TOKEN", "secret"), \
                mock.patch.object(server, "HOST", "0.0.0.0"):
            self.assertFalse(server.Handler._authorized(obj, "/api/leads"))
            obj.headers = {"Authorization": "Bearer secret"}
            self.assertTrue(server.Handler._authorized(obj, "/api/leads"))

    def test_cross_origin_rejected(self):
        server, obj = self._handler({"Origin": "https://evil.example"})
        with mock.patch.object(server, "HOST", "127.0.0.1"), \
                mock.patch.object(server, "PORT", 8787):
            self.assertFalse(server.Handler._origin_allowed(obj))

    def test_same_origin_allowed(self):
        server, obj = self._handler({"Origin": "http://127.0.0.1:8787"})
        with mock.patch.object(server, "HOST", "127.0.0.1"), \
                mock.patch.object(server, "PORT", 8787):
            self.assertTrue(server.Handler._origin_allowed(obj))


class ErasureTests(unittest.TestCase):
    def setUp(self):
        import tempfile
        from pathlib import Path
        import lead_hunter.db as db
        self.db = db
        self._orig = db.DB_PATH
        self._tmp = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(self._tmp.name) / "erasure.db"
        db.initialize()

    def tearDown(self):
        self.db.DB_PATH = self._orig
        self._tmp.cleanup()

    def _insert(self, name: str, *, updated: str, engagement: str = "not_contacted", dnc: int = 0):
        with self.db.connect() as conn:
            cur = conn.execute(
                "INSERT INTO leads (source,source_id,name,country,city,category,"
                "engagement_status,do_not_contact,updated_at) VALUES (?,?,?,?,?,?,?,?,?)",
                ("test", name, name, "Turkey", "Afyonkarahisar", "restaurant",
                 engagement, dnc, updated),
            )
            conn.commit()
            return cur.lastrowid

    def test_delete_lead_removes_row_and_activities(self):
        lead_id = self._insert("Gone", updated="2026-10-05 00:00:00")
        with self.db.connect() as conn:
            conn.execute("INSERT INTO lead_activities (lead_id,kind) VALUES (?,?)", (lead_id, "note"))
            conn.commit()
        self.assertTrue(self.db.delete_lead(lead_id))
        self.assertIsNone(self.db.get_lead(lead_id))
        with self.db.connect() as conn:
            remaining = conn.execute("SELECT COUNT(*) FROM lead_activities WHERE lead_id=?", (lead_id,)).fetchone()[0]
        self.assertEqual(remaining, 0)

    def test_purge_keeps_recent_and_engaged(self):
        old = self._insert("Old", updated="2020-01-01 00:00:00")
        self._insert("OldEngaged", updated="2020-01-01 00:00:00", engagement="contacted")
        self._insert("OldDnc", updated="2020-01-01 00:00:00", dnc=1)
        self._insert("Fresh", updated="2026-10-05 00:00:00")

        purged = self.db.purge_stale_leads(days=30)

        self.assertEqual(purged, 1)
        self.assertIsNone(self.db.get_lead(old))
        with self.db.connect() as conn:
            names = {r[0] for r in conn.execute("SELECT name FROM leads").fetchall()}
        self.assertEqual(names, {"OldEngaged", "OldDnc", "Fresh"})

    def test_purge_rejects_negative_days(self):
        self._insert("Old", updated="2020-01-01 00:00:00")
        with self.assertRaises(ValueError):
            self.db.purge_stale_leads(days=-1)
        with self.db.connect() as conn:
            remaining = conn.execute("SELECT COUNT(*) FROM leads").fetchone()[0]
        self.assertEqual(remaining, 1)


if __name__ == "__main__":
    unittest.main()
