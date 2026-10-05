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
    ("seo_score", "SEO"),
    ("aeo_score", "AEO"),
    ("geo_score", "GEO"),
    ("ai_visibility_score", "AI Visibility"),
    ("opportunity_gap_score", "Opportunity Gap"),
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


SOCIAL_KEYS = ("instagram", "facebook", "linkedin", "x", "youtube", "tiktok", "telegram", "whatsapp")

LEAD_WIDTHS = [8,30,18,18,18,14,14,14,12,12,12,14,16,18,18,34,20,28,28,28,28,28,28,28,28,28,18,18,22,22,22,38,14,20]
SUMMARY_WIDTHS = [44, 14]
SECTOR_WIDTHS = [24,12,12,14,12,12,14,12,14,22]


def _number(value: Any) -> float | None:
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def _average(values: list[float | None]) -> float | None:
    present = [v for v in values if v is not None]
    return round(sum(present) / len(present), 1) if present else None


def _has_contact(row: dict[str, Any]) -> bool:
    flat = _flat_row(row)
    if str(flat.get("email") or "").strip() or str(flat.get("phone") or "").strip():
        return True
    return any(str(flat.get(key) or "").strip() for key in SOCIAL_KEYS)


def _worksheet_xml(matrix: list[list[Any]], *, widths: list[int] | None = None,
                   freeze_header: bool = True, autofilter: bool = False) -> str:
    xml_rows = []
    for r_idx, values in enumerate(matrix, start=1):
        cells = []
        for c_idx, value in enumerate(values, start=1):
            if value is None or value == "":
                continue
            ref = f"{_col_name(c_idx)}{r_idx}"
            if isinstance(value, bool):
                cells.append(f'<c r="{ref}" t="b"><v>{1 if value else 0}</v></c>')
            elif isinstance(value, (int, float)):
                cells.append(f'<c r="{ref}"><v>{value}</v></c>')
            else:
                style = ' s="1"' if freeze_header and r_idx == 1 else ""
                cells.append(f'<c r="{ref}" t="inlineStr"{style}><is><t xml:space="preserve">{_xml_text(value)}</t></is></c>')
        xml_rows.append(f'<row r="{r_idx}">{"".join(cells)}</row>')

    last_col = _col_name(max(1, max((len(v) for v in matrix), default=1)))
    last_row = max(1, len(matrix))
    cols = ""
    if widths:
        cols = "<cols>" + "".join(
            f'<col min="{i}" max="{i}" width="{width}" customWidth="1"/>'
            for i, width in enumerate(widths, start=1)
        ) + "</cols>"
    pane = (
        '<pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/>'
        if freeze_header else ""
    )
    auto = f'<autoFilter ref="A1:{last_col}{last_row}"/>' if autofilter else ""
    return f'''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <dimension ref="A1:{last_col}{last_row}"/>
  <sheetViews><sheetView workbookViewId="0">{pane}</sheetView></sheetViews>
  {cols}
  <sheetData>{"".join(xml_rows)}</sheetData>
  {auto}
</worksheet>'''


def _leads_matrix(rows: list[dict[str, Any]]) -> list[list[Any]]:
    matrix: list[list[Any]] = [[label for _, label in EXPORT_COLUMNS]]
    for raw in rows:
        row = _flat_row(raw)
        matrix.append([row.get(key, "") for key, _ in EXPORT_COLUMNS])
    return matrix


def _summary_matrix(rows: list[dict[str, Any]]) -> list[list[Any]]:
    flat = [_flat_row(r) for r in rows]
    total = len(flat)

    def count(pred) -> int:
        return sum(1 for r in flat if pred(r))

    def status(value: str) -> int:
        return count(lambda r: (r.get("website_status") or "") == value)

    engaged = count(lambda r: (r.get("engagement_status") or "") in {"sent", "delivered", "replied"})
    return [
        ["Metrik", "Değer"],
        ["Toplam işletme", total],
        ["Web üzerinden doğrulanmış", count(lambda r: (r.get("verification_status") or "") in {"verified", "web_verified"})],
        ["Web sitesi yok", status("missing")],
        ["Zayıf web sitesi", status("weak")],
        ["Ölü web sitesi", status("dead")],
        ["Sağlıklı web sitesi", status("healthy")],
        ["E-posta bulundu", count(lambda r: bool(str(r.get("email") or "").strip()))],
        ["Telefon bulundu", count(lambda r: bool(str(r.get("phone") or "").strip()))],
        ["Sosyal profil bulundu", count(lambda r: any(str(r.get(k) or "").strip() for k in SOCIAL_KEYS))],
        ["Ulaşılabilir (e-posta/telefon/sosyal)", count(_has_contact)],
        ["Ortalama fırsat skoru", _average([_number(r.get("lead_score")) for r in flat])],
        ["Yüksek fırsat (60+)", count(lambda r: (_number(r.get("lead_score")) or 0) >= 60)],
        ["Ortalama ticari skor", _average([_number(r.get("commercial_score")) for r in flat])],
        ["Ortalama fırsat boşluğu", _average([_number(r.get("opportunity_gap_score")) for r in flat])],
        ["Ortalama SEO skoru", _average([_number(r.get("seo_score")) for r in flat])],
        ["Ortalama AEO skoru", _average([_number(r.get("aeo_score")) for r in flat])],
        ["Ortalama GEO skoru", _average([_number(r.get("geo_score")) for r in flat])],
        ["Ortalama AI görünürlük skoru", _average([_number(r.get("ai_visibility_score")) for r in flat])],
        ["İletişime geçilmiş", engaged],
        ["Yanıt alınmış", count(lambda r: (r.get("engagement_status") or "") == "replied" or bool(r.get("last_reply_at")))],
        ["İletişime geçilmemiş", total - engaged],
        ["İletişim istenmiyor", count(lambda r: bool(r.get("do_not_contact")))],
    ]


def _sector_decision(size: int, sector_score: float, gap_ratio: float) -> str:
    if size < 3:
        return "Gözlem (Watch)"
    if sector_score >= 65 and gap_ratio >= 0.5:
        return "Öncelikli (Priority)"
    if sector_score >= 50:
        return "Fırsat (Opportunity)"
    if sector_score >= 35:
        return "Takipte (Follow-up)"
    return "Düşük (Low)"


def _sector_matrix(rows: list[dict[str, Any]]) -> list[list[Any]]:
    groups: dict[str, list[dict[str, Any]]] = {}
    for raw in rows:
        row = _flat_row(raw)
        sector = str(row.get("category") or "").strip() or "(bilinmiyor)"
        groups.setdefault(sector, []).append(row)

    matrix: list[list[Any]] = [[
        "Sektör", "İşletme", "Ort. Skor", "Yüksek Fırsat", "Site Yok",
        "Zayıf Site", "Ulaşılabilir", "Ort. Boşluk", "Sektör Skoru", "Karar",
    ]]
    for sector, items in sorted(groups.items(), key=lambda kv: (-len(kv[1]), kv[0])):
        size = len(items)
        avg_score = _average([_number(r.get("lead_score")) for r in items])
        avg_gap = _average([_number(r.get("opportunity_gap_score")) for r in items])
        no_site = sum(1 for r in items if (r.get("website_status") or "") == "missing")
        weak = sum(1 for r in items if (r.get("website_status") or "") == "weak")
        base = avg_score or 0.0
        sector_score = round(base if avg_gap is None else (base + avg_gap) / 2, 1)
        gap_ratio = (no_site + weak) / size if size else 0.0
        matrix.append([
            sector, size, avg_score,
            sum(1 for r in items if (_number(r.get("lead_score")) or 0) >= 60),
            no_site, weak,
            sum(1 for r in items if _has_contact(r)),
            avg_gap, sector_score,
            _sector_decision(size, sector_score, gap_ratio),
        ])
    return matrix


def xlsx_bytes(rows: list[dict[str, Any]]) -> bytes:
    now = datetime.now(timezone.utc).replace(microsecond=0).isoformat()
    output = io.BytesIO()

    sheets = [
        ("Özet", _worksheet_xml(_summary_matrix(rows), widths=SUMMARY_WIDTHS, freeze_header=True)),
        ("SektorKarar", _worksheet_xml(_sector_matrix(rows), widths=SECTOR_WIDTHS, freeze_header=True)),
        ("Leads", _worksheet_xml(_leads_matrix(rows), widths=LEAD_WIDTHS, autofilter=True)),
    ]

    content_types = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/worksheets/sheet3.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
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

    sheet_tags = "".join(
        f'<sheet name="{_xml_text(name)}" sheetId="{i}" r:id="rId{i}"/>'
        for i, (name, _) in enumerate(sheets, start=1)
    )
    workbook = f'''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
 xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>{sheet_tags}</sheets>
</workbook>'''

    sheet_rels = "".join(
        f'<Relationship Id="rId{i}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet{i}.xml"/>'
        for i in range(1, len(sheets) + 1)
    )
    workbook_rels = f'''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  {sheet_rels}
  <Relationship Id="rId{len(sheets) + 1}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
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
        for i, (_, xml) in enumerate(sheets, start=1):
            zf.writestr(f"xl/worksheets/sheet{i}.xml", xml)
        zf.writestr("docProps/core.xml", core)
        zf.writestr("docProps/app.xml", app)

    return output.getvalue()
