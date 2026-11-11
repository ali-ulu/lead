// Package exporters renders leads as CSV and as a native XLSX workbook with
// three sheets: Özet (summary), SektorKarar (sector decision) and Leads.
package exporters

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"strings"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

// ExportColumns pairs a field key with its column label.
var ExportColumns = [][2]string{
	{"id", "ID"}, {"name", "Business"}, {"country", "Country"}, {"city", "City"},
	{"category", "Category"}, {"lead_score", "Opportunity Score"}, {"contactability_score", "Contactability"},
	{"commercial_score", "Commercial Score"}, {"seo_score", "SEO"}, {"aeo_score", "AEO"},
	{"geo_score", "GEO"}, {"ai_visibility_score", "AI Visibility"},
	{"ai_visibility_score_percentile", "AI Visibility Percentile"}, {"opportunity_gap_score", "Opportunity Gap"},
	{"score_confidence", "Score Confidence"}, {"ttfb_ms", "TTFB (ms)"}, {"response_ms", "Response (ms)"},
	{"ai_presence", "AI Presence"}, {"verification_status", "Verification"}, {"website_status", "Website Status"},
	{"website", "Website"}, {"phone", "Phone"}, {"email", "Email"}, {"instagram", "Instagram"},
	{"facebook", "Facebook"}, {"linkedin", "LinkedIn"}, {"x", "X / Twitter"}, {"youtube", "YouTube"},
	{"tiktok", "TikTok"}, {"telegram", "Telegram"}, {"whatsapp", "WhatsApp"},
	{"pipeline_status", "Pipeline Stage"}, {"engagement_status", "Engagement"},
	{"last_contacted_at", "Last Contacted"}, {"last_reply_at", "Last Reply"}, {"follow_up_at", "Follow-up"},
	{"notes", "Notes"}, {"source", "Source"}, {"source_id", "Source ID"},
}

var socialKeys = []string{"instagram", "facebook", "linkedin", "x", "youtube", "tiktok", "telegram", "whatsapp"}

func flatRow(l *model.Lead) map[string]string {
	out := map[string]string{}
	set := func(k string, v any) {
		if v != nil {
			out[k] = fmt.Sprint(v)
		}
	}
	set("id", l.ID)
	set("name", l.Name)
	set("country", l.Country)
	set("city", l.City)
	set("category", l.Category)
	set("lead_score", l.LeadScore)
	set("contactability_score", l.ContactabilityScore)
	set("commercial_score", l.CommercialScore)
	setIntPtr(out, "seo_score", l.SEOScore)
	setIntPtr(out, "aeo_score", l.AEOScore)
	setIntPtr(out, "geo_score", l.GEOScore)
	setIntPtr(out, "ai_visibility_score", l.AIVisibilityScore)
	setIntPtr(out, "ai_visibility_score_percentile", l.AIVisibilityScorePercentile)
	setIntPtr(out, "opportunity_gap_score", l.OpportunityGapScore)
	setIntPtr(out, "ttfb_ms", l.TTFBMS)
	setIntPtr(out, "response_ms", l.ResponseMS)
	if len(l.ScoreConfidence) > 0 {
		out["score_confidence"] = jsonString(l.ScoreConfidence)
	}
	if len(l.AIPresence) > 0 {
		out["ai_presence"] = jsonString(l.AIPresence)
	}
	set("verification_status", l.VerificationStatus)
	set("website_status", l.WebsiteStatus)
	setStrPtr(out, "website", l.Website)
	setStrPtr(out, "phone", l.Phone)
	setStrPtr(out, "email", l.Email)
	for _, k := range socialKeys {
		if v := l.SocialLinks[k]; v != "" {
			out[k] = v
		}
	}
	set("pipeline_status", l.PipelineStatus)
	set("engagement_status", l.EngagementStatus)
	setStrPtr(out, "last_contacted_at", l.LastContactedAt)
	setStrPtr(out, "last_reply_at", l.LastReplyAt)
	setStrPtr(out, "follow_up_at", l.FollowUpAt)
	setStrPtr(out, "notes", l.Notes)
	set("source", l.Source)
	set("source_id", l.SourceID)
	set("audit_engine", strOrEmpty(l.AuditEngine))
	set("do_not_contact", l.DoNotContact)
	return out
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func setIntPtr(m map[string]string, key string, v *int) {
	if v != nil {
		m[key] = fmt.Sprintf("%d", *v)
	}
}

func setStrPtr(m map[string]string, key string, v *string) {
	if v != nil && *v != "" {
		m[key] = *v
	}
}

func jsonString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func safeCSV(value string) string {
	if strings.HasPrefix(value, "=") || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "@") {
		return "'" + value
	}
	return value
}

// CSV renders leads as UTF-8 CSV with a BOM.
func CSV(rows []*model.Lead) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\ufeff")
	w := csv.NewWriter(&buf)
	header := make([]string, len(ExportColumns))
	for i, c := range ExportColumns {
		header[i] = c[1]
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, row := range rows {
		flat := flatRow(row)
		rec := make([]string, len(ExportColumns))
		for i, c := range ExportColumns {
			rec[i] = safeCSV(flat[c[0]])
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// --- XLSX -----------------------------------------------------------------

func colName(index int) string {
	result := ""
	for n := index; n > 0; {
		rem := (n - 1) % 26
		n = (n - 1) / 26
		result = string(rune('A'+rem)) + result
	}
	return result
}

func xmlText(v string) string {
	return html.EscapeString(v)
}

var leadWidths = []int{8, 30, 18, 18, 18, 14, 14, 14, 12, 12, 12, 14, 16, 18, 18, 34, 20, 28, 28, 28, 28, 28, 28, 28, 28, 28, 18, 18, 22, 22, 22, 38, 14, 20}
var summaryWidths = []int{44, 14}
var sectorWidths = []int{24, 12, 12, 14, 12, 12, 14, 12, 14, 22}

func worksheetXML(matrix [][]any, widths []int, freezeHeader, autofilter bool) string {
	var rowsXML strings.Builder
	for rIdx, values := range matrix {
		rowNum := rIdx + 1
		var cells strings.Builder
		for cIdx, value := range values {
			if value == nil || value == "" {
				continue
			}
			ref := fmt.Sprintf("%s%d", colName(cIdx+1), rowNum)
			switch v := value.(type) {
			case bool:
				n := 0
				if v {
					n = 1
				}
				fmt.Fprintf(&cells, `<c r="%s" t="b"><v>%d</v></c>`, ref, n)
			case int:
				fmt.Fprintf(&cells, `<c r="%s"><v>%d</v></c>`, ref, v)
			case float64:
				fmt.Fprintf(&cells, `<c r="%s"><v>%g</v></c>`, ref, v)
			default:
				style := ""
				if freezeHeader && rowNum == 1 {
					style = ` s="1"`
				}
				fmt.Fprintf(&cells, `<c r="%s" t="inlineStr"%s><is><t xml:space="preserve">%s</t></is></c>`, ref, style, xmlText(fmt.Sprint(v)))
			}
		}
		fmt.Fprintf(&rowsXML, `<row r="%d">%s</row>`, rowNum, cells.String())
	}

	lastCol := colName(max(1, maxLen(matrix)))
	lastRow := max(1, len(matrix))
	cols := ""
	if len(widths) > 0 {
		var cb strings.Builder
		for i, w := range widths {
			fmt.Fprintf(&cb, `<col min="%d" max="%d" width="%d" customWidth="1"/>`, i+1, i+1, w)
		}
		cols = "<cols>" + cb.String() + "</cols>"
	}
	pane := ""
	if freezeHeader {
		pane = `<pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/>`
	}
	auto := ""
	if autofilter {
		auto = fmt.Sprintf(`<autoFilter ref="A1:%s%d"/>`, lastCol, lastRow)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <dimension ref="A1:%s%d"/>
  <sheetViews><sheetView workbookViewId="0">%s</sheetView></sheetViews>
  %s
  <sheetData>%s</sheetData>
  %s
</worksheet>`, lastCol, lastRow, pane, cols, rowsXML.String(), auto)
}

func maxLen(matrix [][]any) int {
	m := 0
	for _, row := range matrix {
		if len(row) > m {
			m = len(row)
		}
	}
	return m
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func leadsMatrix(rows []*model.Lead) [][]any {
	matrix := [][]any{}
	header := make([]any, len(ExportColumns))
	for i, c := range ExportColumns {
		header[i] = c[1]
	}
	matrix = append(matrix, header)
	for _, row := range rows {
		flat := flatRow(row)
		rec := make([]any, len(ExportColumns))
		for i, c := range ExportColumns {
			rec[i] = flat[c[0]]
		}
		matrix = append(matrix, rec)
	}
	return matrix
}

func number(v string) (float64, bool) {
	if v == "" {
		return 0, false
	}
	var f float64
	if _, err := fmt.Sscanf(v, "%g", &f); err != nil {
		return 0, false
	}
	return f, true
}

func average(values []float64, present []bool) any {
	var sum float64
	n := 0
	for i, v := range values {
		if present[i] {
			sum += v
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return math.RoundToEven(sum/float64(n)*10) / 10
}

func hasContact(flat map[string]string) bool {
	if strings.TrimSpace(flat["email"]) != "" || strings.TrimSpace(flat["phone"]) != "" {
		return true
	}
	for _, k := range socialKeys {
		if strings.TrimSpace(flat[k]) != "" {
			return true
		}
	}
	return false
}

func aiPresenceFlag(v string, field string) bool {
	if v == "" {
		return false
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(v), &parsed); err != nil {
		return false
	}
	switch field {
	case "measured":
		b, _ := parsed["measured"].(bool)
		return b
	case "cited":
		f, ok := parsed["cited_count"].(float64)
		return ok && f != 0
	}
	return false
}

func summaryMatrix(rows []*model.Lead) [][]any {
	flats := make([]map[string]string, len(rows))
	for i, r := range rows {
		flats[i] = flatRow(r)
	}
	total := len(flats)
	count := func(pred func(map[string]string) bool) int {
		n := 0
		for _, f := range flats {
			if pred(f) {
				n++
			}
		}
		return n
	}
	status := func(v string) int { return count(func(f map[string]string) bool { return f["website_status"] == v }) }
	engaged := count(func(f map[string]string) bool {
		return f["engagement_status"] == "sent" || f["engagement_status"] == "delivered" || f["engagement_status"] == "replied"
	})

	collect := func(field string) ([]float64, []bool) {
		vals := make([]float64, len(flats))
		pres := make([]bool, len(flats))
		for i, f := range flats {
			if v, ok := number(f[field]); ok {
				vals[i] = v
				pres[i] = true
			}
		}
		return vals, pres
	}
	avgScoreV, avgScoreP := collect("lead_score")
	avgCommV, avgCommP := collect("commercial_score")
	avgGapV, avgGapP := collect("opportunity_gap_score")
	avgSEOV, avgSEOP := collect("seo_score")
	avgAEOV, avgAEOP := collect("aeo_score")
	avgGEOV, avgGEOP := collect("geo_score")
	avgAIOV, avgAIOP := collect("ai_visibility_score")

	return [][]any{
		{"Metrik", "Değer"},
		{"Toplam işletme", total},
		{"Web üzerinden doğrulanmış", count(func(f map[string]string) bool {
			return f["verification_status"] == "verified" || f["verification_status"] == "web_verified"
		})},
		{"Web sitesi yok", status("missing")},
		{"Zayıf web sitesi", status("weak")},
		{"Ölü web sitesi", status("dead")},
		{"Sağlıklı web sitesi", status("healthy")},
		{"E-posta bulundu", count(func(f map[string]string) bool { return strings.TrimSpace(f["email"]) != "" })},
		{"Telefon bulundu", count(func(f map[string]string) bool { return strings.TrimSpace(f["phone"]) != "" })},
		{"Sosyal profil bulundu", count(func(f map[string]string) bool {
			for _, k := range socialKeys {
				if strings.TrimSpace(f[k]) != "" {
					return true
				}
			}
			return false
		})},
		{"Ulaşılabilir (e-posta/telefon/sosyal)", count(hasContact)},
		{"Ortalama fırsat skoru", average(avgScoreV, avgScoreP)},
		{"Yüksek fırsat (60+)", count(func(f map[string]string) bool { v, ok := number(f["lead_score"]); return ok && v >= 60 })},
		{"Ortalama ticari skor", average(avgCommV, avgCommP)},
		{"Ortalama fırsat boşluğu", average(avgGapV, avgGapP)},
		{"Ortalama SEO skoru", average(avgSEOV, avgSEOP)},
		{"Ortalama AEO skoru", average(avgAEOV, avgAEOP)},
		{"Ortalama GEO skoru", average(avgGEOV, avgGEOP)},
		{"Ortalama AI görünürlük skoru", average(avgAIOV, avgAIOP)},
		{"Lighthouse ile ölçülen", count(func(f map[string]string) bool { return strings.Contains(f["audit_engine"], "lighthouse") })},
		{"AI varlığı ölçülen", count(func(f map[string]string) bool { return aiPresenceFlag(f["ai_presence"], "measured") })},
		{"AI yanıtlarında geçen", count(func(f map[string]string) bool { return aiPresenceFlag(f["ai_presence"], "cited") })},
		{"İletişime geçilmiş", engaged},
		{"Yanıt alınmış", count(func(f map[string]string) bool {
			return f["engagement_status"] == "replied" || strings.TrimSpace(f["last_reply_at"]) != ""
		})},
		{"İletişime geçilmemiş", total - engaged},
		{"İletişim istenmiyor", count(func(f map[string]string) bool { return f["do_not_contact"] == "true" || f["do_not_contact"] == "1" })},
	}
}

func sectorDecision(size int, sectorScore, gapRatio float64) string {
	if size < 3 {
		return "Gözlem (Watch)"
	}
	if sectorScore >= 65 && gapRatio >= 0.5 {
		return "Öncelikli (Priority)"
	}
	if sectorScore >= 50 {
		return "Fırsat (Opportunity)"
	}
	if sectorScore >= 35 {
		return "Takipte (Follow-up)"
	}
	return "Düşük (Low)"
}

func sectorMatrix(rows []*model.Lead) [][]any {
	groups := map[string][]map[string]string{}
	for _, r := range rows {
		flat := flatRow(r)
		sector := strings.TrimSpace(flat["category"])
		if sector == "" {
			sector = "(bilinmiyor)"
		}
		groups[sector] = append(groups[sector], flat)
	}
	type kv struct {
		sector string
		items  []map[string]string
	}
	var sorted []kv
	for s, items := range groups {
		sorted = append(sorted, kv{s, items})
	}
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0; j-- {
			less := len(sorted[j].items) > len(sorted[j-1].items) ||
				(len(sorted[j].items) == len(sorted[j-1].items) && sorted[j].sector < sorted[j-1].sector)
			if less {
				sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
			} else {
				break
			}
		}
	}

	matrix := [][]any{{
		"Sektör", "İşletme", "Ort. Skor", "Yüksek Fırsat", "Site Yok",
		"Zayıf Site", "Ulaşılabilir", "Ort. Boşluk", "Sektör Skoru", "Karar",
	}}
	for _, g := range sorted {
		items := g.items
		size := len(items)
		scoreV := make([]float64, size)
		scoreP := make([]bool, size)
		gapV := make([]float64, size)
		gapP := make([]bool, size)
		noSite, weak, highOpp, contactable := 0, 0, 0, 0
		for i, f := range items {
			if v, ok := number(f["lead_score"]); ok {
				scoreV[i] = v
				scoreP[i] = true
				if v >= 60 {
					highOpp++
				}
			}
			if v, ok := number(f["opportunity_gap_score"]); ok {
				gapV[i] = v
				gapP[i] = true
			}
			if f["website_status"] == "missing" {
				noSite++
			}
			if f["website_status"] == "weak" {
				weak++
			}
			if hasContact(f) {
				contactable++
			}
		}
		avgScore := average(scoreV, scoreP)
		avgGap := average(gapV, gapP)
		base := 0.0
		if v, ok := avgScore.(float64); ok {
			base = v
		}
		sectorScore := base
		if v, ok := avgGap.(float64); ok {
			sectorScore = math.RoundToEven((base+v)/2*10) / 10
		}
		gapRatio := 0.0
		if size > 0 {
			gapRatio = float64(noSite+weak) / float64(size)
		}
		matrix = append(matrix, []any{
			g.sector, size, avgScore, highOpp, noSite, weak, contactable, avgGap, sectorScore,
			sectorDecision(size, sectorScore, gapRatio),
		})
	}
	return matrix
}

// XLSX renders leads as a three-sheet workbook.
func XLSX(rows []*model.Lead) []byte {
	now := time.Now().UTC().Truncate(time.Second).Format("2006-01-02T15:04:05")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	type sheet struct {
		name string
		xml  string
	}
	sheets := []sheet{
		{"Özet", worksheetXML(summaryMatrix(rows), summaryWidths, true, false)},
		{"SektorKarar", worksheetXML(sectorMatrix(rows), sectorWidths, true, false)},
		{"Leads", worksheetXML(leadsMatrix(rows), leadWidths, false, true)},
	}

	write := func(name, content string) {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(content))
	}

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
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
</Types>`
	rootRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

	var sheetTags strings.Builder
	for i, s := range sheets {
		fmt.Fprintf(&sheetTags, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlText(s.name), i+1, i+1)
	}
	workbook := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
 xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>%s</sheets>
</workbook>`, sheetTags.String())

	var sheetRels strings.Builder
	for i := range sheets {
		fmt.Fprintf(&sheetRels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i+1, i+1)
	}
	workbookRels := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  %s
  <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`, sheetRels.String(), len(sheets)+1)

	styles := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
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
</styleSheet>`

	core := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
 xmlns:dc="http://purl.org/dc/elements/1.1/"
 xmlns:dcterms="http://purl.org/dc/terms/"
 xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:creator>LeadScout</dc:creator>
  <cp:lastModifiedBy>LeadScout</cp:lastModifiedBy>
  <dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created>
  <dcterms:modified xsi:type="dcterms:W3CDTF">%s</dcterms:modified>
</cp:coreProperties>`, now, now)

	app := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"
 xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">
  <Application>LeadScout</Application>
</Properties>`

	write("[Content_Types].xml", contentTypes)
	write("_rels/.rels", rootRels)
	write("xl/workbook.xml", workbook)
	write("xl/_rels/workbook.xml.rels", workbookRels)
	write("xl/styles.xml", styles)
	for i, s := range sheets {
		write(fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), s.xml)
	}
	write("docProps/core.xml", core)
	write("docProps/app.xml", app)

	_ = zw.Close()
	return buf.Bytes()
}
