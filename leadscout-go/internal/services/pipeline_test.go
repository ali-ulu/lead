package services

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/exporters"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/osm"
)

// Full pipeline: discovery (mocked Overpass) -> normalize -> persist -> query -> export.
// Overture is unavailable in the Go build, so only the OSM provider is exercised.
func TestDiscoverToExportPipeline(t *testing.T) {
	useTempDB(t)

	payload := `{"elements":[
		{"type":"node","id":101,"lat":41.01,"lon":28.97,"tags":{"name":"Alpha Cafe","amenity":"cafe","website":"https://alpha.example"}},
		{"type":"node","id":102,"lat":41.02,"lon":28.98,"tags":{"name":"Beta Cafe","amenity":"cafe"}}
	]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	original := osm.OverpassURLs
	osm.OverpassURLs = []string{server.URL}
	defer func() { osm.OverpassURLs = original }()

	max := 10
	result, err := DiscoverBusinesses("Istanbul", "cafe", "Turkey", 3, &max, []string{"osm"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if result.Count != 2 {
		t.Fatalf("expected 2 discovered leads, got %d (%+v)", result.Count, result)
	}

	rows, err := QueryLeads(QueryFilters{Category: "cafe"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 stored leads, got %d", len(rows))
	}
	for _, r := range rows {
		if r.LeadScore == 0 {
			t.Errorf("lead %q was not scored", r.Name)
		}
	}

	data, err := exporters.CSV(rows)
	if err != nil {
		t.Fatalf("csv: %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) != 3 { // header + 2 rows
		t.Fatalf("expected 3 csv records, got %d", len(records))
	}
}
