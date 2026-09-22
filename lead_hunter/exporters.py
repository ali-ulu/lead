from __future__ import annotations

import csv
import io
import json
import zipfile
from datetime import datetime, timezone
from html import escape
from typing import Any

EXPORT_COLUMNS = [
    ("id", "ID"),
    ("name", "Business"),
    ("country", "Country"),
    ("city", "City"),
    ("category", "Category"),
    ("lead_score", "Opportunity Score"),
    ("contactability_score", "Contactability"),
    ("commercial_score", "Commercial Score"),
    ("verification_status", "Verification"),
    ("website_status", "Website Status"),
    ("website", "Website"),
    ("phone", "Phone"),
    ("email", "Email"),
    ("instagram", "Instagram"),
    ("facebook", "Facebook"),
    ("linkedin", "LinkedIn"),
    ("x", "X / Twitter"),
    ("youtube", "YouTube"),
    ("tiktok", "TikTok"),
    ("telegram", "Telegram"),
    ("whatsapp", "WhatsApp"),
    ("pipeline_status", "Pipeline Stage"),
    ("engagement_status", "Engagement"),
    ("last_contacted_at", "Last Contacted"),
    ("last_reply_at", "Last Reply"),
    ("follow_up_at", "Follow-up"),
    ("notes", "Notes"),
    ("source", "Source"),
    ("source_id", "Source ID"),
]


def _flat_row(row: dict[str, Any]) -> dict[str, Any]:
    socials = row.get("social_links") or {}
    if isinstance(socials, str):
        try:
            socials = json.loads(socials)
        except Exception:
            socials = {}
    flat = dict(row)
    for platform in ("instagram", "facebook", "linkedin", "x", "youtube", "tiktok", "telegram", "whatsapp"):
        flat[platform] = socials.get(platform, "")
    return flat


def _safe_csv(value: Any) -> str:
    text = "" if value is None else str(value)
    if text.startswith(("=", "+", "-", "@")):
        return "'" + text
    return text


def csv_bytes(rows: list[dict[str, Any]]) -> bytes:
    buf = io.StringIO()
    headers = [label for _, label in EXPORT_COLUMNS]
    writer = csv.writer(buf)
    writer.writerow(headers)
    for raw in rows:
        row = _flat_row(raw)
        writer.writerow([_safe_csv(row.get(key, "")) for key, _ in EXPORT_COLUMNS])
    return buf.getvalue().encode("utf-8-sig")


def _col_name(index: int) -> str:
    result = ""
    n = index
    while n:
        n, rem = divmod(n - 1, 26)
        result = chr(65 + rem) + result
    return result


def _xml_text(value: Any) -> str:
    return escape("" if value is None else str(value), quote=False)


def _sheet_xml(rows: list[dict[str, Any]]) -> str:
    matrix: list[list[Any]] = [[label for _, label in EXPORT_COLUMNS]]
    for raw in rows:
        row = _flat_row(raw)
        matrix.append([row.get(key, "") for key, _ in EXPORT_COLUMNS])

    xml_rows = []
    for r_idx, values in enumerate(matrix, start=1):
        cells = []
        for c_idx, value in enumerate(values, start=1):
            ref = f"{_col_name(c_idx)}{r_idx}"
            if isinstance(value, bool):
                cells.append(f'<c r="{ref}" t="b"><v>{1 if value else 0}</v></c>')
            elif isinstance(value, (int, float)) and not isinstance(value, bool):
                cells.append(f'<c r="{ref}"><v>{value}</v></c>')
            else:
                style = ' s="1"' if r_idx == 1 else ""
                text = _xml_text(value)
                cells.append(f'<c r="{ref}" t="inlineStr"{style}><is><t xml:space="preserve">{text}</t></is></c>')
        xml_rows.append(f'<row r="{r_idx}">{"".join(cells)}</row>')

    last_col = _col_name(len(EXPORT_COLUMNS))
    last_row = max(1, len(matrix))
    widths = [8,30,18,18,18,14,14,14,18,18,34,20,28,28,28,28,28,28,28,28,28,18,18,22,22,22,38,14,20]
    cols = "".join(
        f'<col min="{i}" max="{i}" width="{width}" customWidth="1"/>'
        for i, width in enumerate(widths, start=1)
    )
    return f'''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <dimension ref="A1:{last_col}{last_row}"/>
  <sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>
  <cols>{cols}</cols>
  <sheetData>{"".join(xml_rows)}</sheetData>
  <autoFilter ref="A1:{last_col}{last_row}"/>
</worksheet>'''


def xlsx_bytes(rows: list[dict[str, Any]]) -> bytes:
    now = datetime.now(timezone.utc).replace(microsecond=0).isoformat()
    output = io.BytesIO()

    content_types = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>'''

    root_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>'''

    workbook = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
 xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets><sheet name="Leads" sheetId="1" r:id="rId1"/></sheets>
</workbook>'''

    workbook_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>'''

    styles = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="2">
    <font><sz val="11"/><name val="Aptos"/></font>
    <font><b/><color rgb="FFFFFFFF"/><sz val="11"/><name val="Aptos"/></font>
  </fonts>
  <fills count="3">
    <fill><patternFill patternType="none"/></fill>
    <fill><patternFill patternType="gray125"/></fill>
    <fill><patternFill patternType="solid"><fgColor rgb="FFB31224"/><bgColor indexed="64"/></patternFill></fill>
  </fills>
  <borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="2">
    <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
    <xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1"/>
  </cellXfs>
  <cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>'''

    core = f'''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
 xmlns:dc="http://purl.org/dc/elements/1.1/"
 xmlns:dcterms="http://purl.org/dc/terms/"
 xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:creator>LeadScout</dc:creator>
  <cp:lastModifiedBy>LeadScout</cp:lastModifiedBy>
  <dcterms:created xsi:type="dcterms:W3CDTF">{now}</dcterms:created>
  <dcterms:modified xsi:type="dcterms:W3CDTF">{now}</dcterms:modified>
</cp:coreProperties>'''

    app = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"
 xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">
  <Application>LeadScout</Application>
</Properties>'''

    with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("[Content_Types].xml", content_types)
        zf.writestr("_rels/.rels", root_rels)
        zf.writestr("xl/workbook.xml", workbook)
        zf.writestr("xl/_rels/workbook.xml.rels", workbook_rels)
        zf.writestr("xl/styles.xml", styles)
        zf.writestr("xl/worksheets/sheet1.xml", _sheet_xml(rows))
        zf.writestr("docProps/core.xml", core)
        zf.writestr("docProps/app.xml", app)

    return output.getvalue()
