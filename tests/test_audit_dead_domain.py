"""Regression tests for dead-domain handling in audit_url.

Overture still lists retired domains. _assert_public_host raised ValueError on
an unresolvable hostname, the surrounding except clause only caught
URLError/TimeoutError/SSLError, and the exception escaped: the lead was never
updated and a batch run aborted on the first stale record. A dead domain is a
finding about the business, not an error.
"""
import unittest
from unittest import mock

import lead_hunter.audit as audit


class DeadDomainTests(unittest.TestCase):
    def test_unresolvable_host_is_reported_as_dead(self):
        import socket

        with mock.patch.object(audit.socket, "getaddrinfo",
                               side_effect=socket.gaierror("Name or service not known")):
            result = audit.audit_url("http://retired-domain.example/")

        self.assertFalse(result["reachable"])
        self.assertEqual(result["website_status"], "dead")
        self.assertIn("could not be resolved", result["error"])
        self.assertEqual(result["social_links"], {})

    def test_private_host_still_raises(self):
        """Only DNS failures become 'dead'; safety checks keep raising."""
        import socket

        infos = [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("127.0.0.1", 80))]
        with mock.patch.object(audit.socket, "getaddrinfo", return_value=infos):
            with self.assertRaises(ValueError) as ctx:
                audit.audit_url("http://127.0.0.1/")

        self.assertIn("Local/private", str(ctx.exception))

    def test_dead_status_survives_rediscovery(self):
        import sqlite3
        import tempfile
        from contextlib import closing
        from pathlib import Path

        import lead_hunter.db as db

        original = db.DB_PATH
        tempdir = tempfile.TemporaryDirectory()
        db.DB_PATH = Path(tempdir.name) / "dead-test.db"
        try:
            db.initialize()
            row = {
                "source": "test", "source_id": "dead-1", "name": "Retired",
                "country": "Turkey", "city": "Konya", "category": "hairdresser",
                "website": "http://retired.example/", "website_status": "unknown",
            }
            with closing(db.connect()) as conn:
                conn.execute(
                    "UPDATE leads SET website_status='dead' WHERE source_id='dead-1'"
                ) if conn.execute(
                    "SELECT COUNT(*) FROM leads"
                ).fetchone()[0] else conn.execute(
                    "INSERT INTO leads (source,source_id,name,country,city,category,"
                    "website,website_status) VALUES (?,?,?,?,?,?,?,'dead')",
                    (row["source"], row["source_id"], row["name"], row["country"],
                     row["city"], row["category"], row["website"]),
                )
                conn.commit()

            db.upsert_leads([dict(row)])
            with closing(db.connect()) as conn:
                found = dict(conn.execute(
                    "SELECT website_status FROM leads WHERE source_id='dead-1'"
                ).fetchone())

            self.assertEqual(found["website_status"], "dead")
        finally:
            db.DB_PATH = original
            tempdir.cleanup()


if __name__ == "__main__":
    unittest.main()