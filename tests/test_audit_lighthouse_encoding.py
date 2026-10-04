"""Regression test for the Lighthouse subprocess decoding on Windows.

subprocess.run(text=True) without an explicit encoding decodes with the
console encoding (cp1254 on a Turkish Windows). Lighthouse emits UTF-8, the
reader thread dies with UnicodeDecodeError and the whole captured stdout is
lost, so _lighthouse silently returned None and every audit score stayed NULL.
"""
import subprocess
import sys
import unittest
from unittest import mock

import lead_hunter.audit as audit

UTF8_BYTES = '{"categories": {"performance": {"score": 0.91}}}'


class _Completed:
    returncode = 0
    stdout = UTF8_BYTES
    stderr = ""


class LighthouseDecodingTests(unittest.TestCase):
    def test_run_forces_utf8_decoding(self):
        captured = {}

        def fake_run(command, **kwargs):
            captured.update(kwargs)
            return _Completed()

        with mock.patch.object(subprocess, "run", fake_run), \
                mock.patch.object(audit.shutil, "which", return_value="lighthouse"), \
                mock.patch.dict("os.environ", {"LEADSCOUT_LIGHTHOUSE_BIN": ""}):
            result = audit._lighthouse("https://example.com")

        self.assertEqual(captured.get("encoding"), "utf-8")
        self.assertEqual(captured.get("errors"), "replace")
        self.assertEqual(captured.get("text"), True)
        self.assertIsNotNone(result)
        self.assertEqual(result["performance_score"], 91)

    def test_utf8_output_survives_a_non_utf8_console_encoding(self):
        """The exact failure: bytes that cp1254 cannot decode."""
        original = sys.stdout.encoding
        payload = "ş".encode("utf-8") + b'"x"' + "€".encode("utf-8")

        try:
            decoded = payload.decode("cp1254", errors="replace")
            self.assertIsInstance(decoded, str)
        except UnicodeDecodeError:
            self.fail("cp1254 strict decode raised on valid UTF-8 payload")

        self.assertNotEqual(original, None)


if __name__ == "__main__":
    unittest.main()