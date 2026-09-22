import io
import unittest
import zipfile
from xml.etree import ElementTree as ET

from lead_hunter.exporters import csv_bytes, xlsx_bytes


SAMPLE = [{
    "id": 1,
    "name": "=Formula-looking business",
    "country": "Pakistan",
    "city": "Karachi",
    "category": "beauty",
    "lead_score": 70,
    "website_status": "missing",
    "website": "",
    "phone": "+923001234567",
    "email": "",
    "social_links": {
        "instagram": "https://instagram.com/example",
        "whatsapp": "https://wa.me/923001234567",
    },
    "pipeline_status": "new",
    "source": "test",
    "source_id": "1",
}]


class ExporterTests(unittest.TestCase):
    def test_csv_has_bom_and_formula_guard(self):
        data = csv_bytes(SAMPLE)
        text = data.decode("utf-8-sig")
        self.assertIn("Business", text.splitlines()[0])
        self.assertIn("'=Formula-looking business", text)

    def test_xlsx_is_valid_openxml_zip(self):
        data = xlsx_bytes(SAMPLE)
        with zipfile.ZipFile(io.BytesIO(data)) as zf:
            names = set(zf.namelist())
            self.assertIn("[Content_Types].xml", names)
            self.assertIn("xl/workbook.xml", names)
            self.assertIn("xl/worksheets/sheet1.xml", names)
            ET.fromstring(zf.read("xl/workbook.xml"))
            sheet = ET.fromstring(zf.read("xl/worksheets/sheet1.xml"))
            self.assertTrue(sheet.tag.endswith("worksheet"))
            self.assertIn(b"Instagram", zf.read("xl/worksheets/sheet1.xml"))


if __name__ == "__main__":
    unittest.main()
