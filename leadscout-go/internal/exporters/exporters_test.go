package exporters

import (
	"archive/zip"
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

func sample() []*model.Lead {
	return []*model.Lead{{
		ID: 1, Name: "=Formula-looking business", Country: "Pakistan", City: "Karachi",
		Category: "beauty", LeadScore: 70, WebsiteStatus: "missing", Website: model.Str(""),
		Phone: model.Str("+923001234567"), Email: model.Str(""),
		SocialLinks: map[string]string{
			"instagram": "https://instagram.com/example",
			"whatsapp":  "https://wa.me/923001234567",
		},
		PipelineStatus: "new", Source: "test", SourceID: "1",
	}}
}

func TestCSVHasBOMAndFormulaGuard(t *testing.T) {
	data, err := CSV(sample())
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	firstLine := strings.SplitN(text, "\n", 2)[0]
	if !strings.Contains(firstLine, "Business") {
		t.Errorf("header missing: %q", firstLine)
	}
	if !strings.Contains(text, "'=Formula-looking business") {
		t.Errorf("formula guard missing")
	}
}

func TestXLSXIsValidOpenXMLZip(t *testing.T) {
	data := XLSX(sample())
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"[Content_Types].xml", "xl/workbook.xml", "xl/worksheets/sheet1.xml", "xl/worksheets/sheet3.xml"} {
		if !names[want] {
			t.Errorf("missing %s", want)
		}
	}
	sheet3 := readZip(t, zr, "xl/worksheets/sheet3.xml")
	if !strings.Contains(sheet3, "Instagram") {
		t.Errorf("sheet3 missing social column")
	}
}

func TestXLSXHasSummaryAndSectorSheetsWithTextLabels(t *testing.T) {
	data := XLSX(sample())
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	workbook := readZip(t, zr, "xl/workbook.xml")
	names := regexp.MustCompile(`<sheet name="([^"]+)"`).FindAllStringSubmatch(workbook, -1)
	var got []string
	for _, m := range names {
		got = append(got, m[1])
	}
	want := []string{"Özet", "SektorKarar", "Leads"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("sheet names = %v, want %v", got, want)
	}
	summary := readZip(t, zr, "xl/worksheets/sheet1.xml")
	if !strings.Contains(summary, `t="inlineStr"`) {
		t.Errorf("summary labels not inline strings")
	}
	if !strings.Contains(summary, "Toplam işletme") || !strings.Contains(summary, "Ortalama fırsat skoru") {
		t.Errorf("summary labels missing")
	}
	sector := readZip(t, zr, "xl/worksheets/sheet2.xml")
	for _, want := range []string{"Sektör", "Karar", "beauty"} {
		if !strings.Contains(sector, want) {
			t.Errorf("sector sheet missing %q", want)
		}
	}
}

func TestSummaryCountsAndAverages(t *testing.T) {
	rows := []*model.Lead{
		{Category: "dentist", LeadScore: 70, OpportunityGapScore: model.Int(80), WebsiteStatus: "missing", Email: model.Str("a@x.de")},
		{Category: "dentist", LeadScore: 40, OpportunityGapScore: model.Int(30), WebsiteStatus: "weak", Phone: model.Str("+49")},
		{Category: "dentist", LeadScore: 62, OpportunityGapScore: model.Int(70), WebsiteStatus: "missing"},
		{Category: "cafe", LeadScore: 50, OpportunityGapScore: model.Int(55), WebsiteStatus: "healthy"},
	}
	data := XLSX(rows)
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	summary := readZip(t, zr, "xl/worksheets/sheet1.xml")
	sector := readZip(t, zr, "xl/worksheets/sheet2.xml")

	if !strings.Contains(summary, `<t xml:space="preserve">Toplam işletme</t></is></c><c r="B2"><v>4</v>`) {
		t.Errorf("total count cell wrong")
	}
	if !strings.Contains(summary, `<t xml:space="preserve">Web sitesi yok</t></is></c><c r="B4"><v>2</v>`) {
		t.Errorf("missing-site count cell wrong")
	}
	if !strings.Contains(sector, ">57.3<") {
		t.Errorf("avg score 57.3 missing")
	}
	if !strings.Contains(sector, ">58.6<") {
		t.Errorf("sector score 58.6 missing")
	}
	if !strings.Contains(sector, "Fırsat (Opportunity)") {
		t.Errorf("opportunity decision missing")
	}
	if !strings.Contains(sector, "Gözlem (Watch)") {
		t.Errorf("watch decision missing")
	}
}

func readZip(t *testing.T, zr *zip.Reader, name string) string {
	t.Helper()
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			data, _ := io.ReadAll(rc)
			return string(data)
		}
	}
	t.Fatalf("zip entry %s not found", name)
	return ""
}
