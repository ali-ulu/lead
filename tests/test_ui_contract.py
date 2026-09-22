import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class UIContractTests(unittest.TestCase):
    def test_javascript_required_ids_exist_in_html(self):
        html = (ROOT / "static" / "index.html").read_text(encoding="utf-8")
        js = (ROOT / "static" / "app.js").read_text(encoding="utf-8")

        html_ids = set(re.findall(r'\bid=["\']([^"\']+)["\']', html))
        js_ids = set(re.findall(r"\$\(['\"]([^'\"]+)['\"]\)", js))

        missing = sorted(js_ids - html_ids)
        self.assertEqual(missing, [], f"JavaScript references missing HTML ids: {missing}")

    def test_excel_and_csv_buttons_are_present(self):
        html = (ROOT / "static" / "index.html").read_text(encoding="utf-8")
        self.assertIn('id="exportXlsx"', html)
        self.assertIn('id="exportCsv"', html)


if __name__ == "__main__":
    unittest.main()
