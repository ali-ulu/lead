package osm

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// End-to-end through the provider: a real Overpass JSON payload must be parsed
// and normalized into leads. The public Overpass mirrors are frequently
// rate-limited, so this pins the behavior against a local server.
func TestSearchAroundProducesLeadsFromOverpassPayload(t *testing.T) {
	payload := `{"elements":[
		{"type":"node","id":1,"lat":41.01,"lon":28.97,"tags":{"name":"Kahve Durağı","amenity":"cafe","website":"https://kahve.example","phone":"+90 212 000 0000"}},
		{"type":"way","id":2,"center":{"lat":41.02,"lon":28.98},"tags":{"name":"Mokka Cafe","amenity":"cafe","contact:instagram":"mokka"}},
		{"type":"node","id":3,"lat":41.03,"lon":28.99,"tags":{"amenity":"cafe"}}
	]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	original := OverpassURLs
	OverpassURLs = []string{server.URL}
	defer func() { OverpassURLs = original }()

	rows, err := SearchAround(41.0082, 28.9784, 3, "cafe", "Istanbul", "Turkey", 10, 10)
	if err != nil {
		t.Fatalf("SearchAround error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 named leads (unnamed node skipped), got %d", len(rows))
	}
	byName := map[string]bool{}
	for _, r := range rows {
		byName[r.Name] = true
		if r.City != "Istanbul" {
			t.Errorf("lead %q city = %q", r.Name, r.City)
		}
		if r.SourceID == "" {
			t.Errorf("lead %q missing source id", r.Name)
		}
	}
	if !byName["Kahve Durağı"] || !byName["Mokka Cafe"] {
		t.Errorf("unexpected lead names: %v", byName)
	}
}
