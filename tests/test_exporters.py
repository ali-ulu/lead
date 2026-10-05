import io
import re
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
            self.assertIn(b"Instagram", zf.read("xl/worksheets/sheet3.xml"))

    def test_xlsx_has_summary_and_sector_sheets_with_text_labels(self):
        data = xlsx_bytes(SAMPLE)
        with zipfile.ZipFile(io.BytesIO(data)) as zf:
            workbook = zf.read("xl/workbook.xml").decode("utf-8")
            names = re.findall(r'<sheet name="([^"]+)"', workbook)
            self.assertEqual(names, ["Özet", "SektorKarar", "Leads"])
            for index in (1, 2, 3):
                self.assertIn(f"xl/worksheets/sheet{index}.xml", zf.namelist())

            # Labels must be written as text cells, not silently dropped while
            # only the numeric values survive.
            summary = zf.read("xl/worksheets/sheet1.xml").decode("utf-8")
            self.assertIn('t="inlineStr"', summary)
            self.assertIn("Toplam işletme", summary)
            self.assertIn("Ortalama fırsat skoru", summary)

            sector = zf.read("xl/worksheets/sheet2.xml").decode("utf-8")
            self.assertIn("Sektör", sector)
            self.assertIn("Karar", sector)
            self.assertIn("beauty", sector)

    def test_summary_counts_and_averages(self):
        rows = [
            {"category": "dentist", "lead_score": 70, "opportunity_gap_score": 80,
             "website_status": "missing", "email": "a@x.de"},
            {"category": "dentist", "lead_score": 40, "opportunity_gap_score": 30,
             "website_status": "weak", "phone": "+49"},
            {"category": "dentist", "lead_score": 62, "opportunity_gap_score": 70,
             "website_status": "missing"},
            {"category": "cafe", "lead_score": 50, "opportunity_gap_score": 55,
             "website_status": "healthy"},
        ]
        with zipfile.ZipFile(io.BytesIO(xlsx_bytes(rows))) as zf:
            summary = zf.read("xl/worksheets/sheet1.xml").decode("utf-8")
            sector = zf.read("xl/worksheets/sheet2.xml").decode("utf-8")
        self.assertIn("<t xml:space=\"preserve\">Toplam işletme</t></is></c><c r=\"B2\"><v>4</v>", summary)
        self.assertIn("<t xml:space=\"preserve\">Web sitesi yok</t></is></c><c r=\"B4\"><v>2</v>", summary)
        # dentist: 3 leads, avg (70+40+62)/3=57.3, sector score (57.3+60)/2=58.6
        self.assertIn(">57.3<", sector)
        self.assertIn(">58.6<", sector)
        self.assertIn("Fırsat (Opportunity)", sector)
        self.assertIn("Gözlem (Watch)", sector)


if __name__ == "__main__":
    unittest.main()
