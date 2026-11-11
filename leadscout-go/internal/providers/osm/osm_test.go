package osm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A gateway error page (Overpass returns plain-text 502 bodies under load) must
// surface as an HTTP error, not as a JSON parse failure on the error page.
func TestFetchOneRejectsHTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("# 502 Gateway Timeout\n\nThis Overpass API server is currently at capacity."))
	}))
	defer server.Close()

	_, err := fetchOne(server.URL, "[out:json];out;", 5)
	if err == nil {
		t.Fatal("expected an error for HTTP 502")
	}
	if !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("error should mention the HTTP status, got: %v", err)
	}
	if strings.Contains(err.Error(), "invalid character") {
		t.Errorf("error should not be a JSON parse failure, got: %v", err)
	}
}

func TestFetchOneParsesSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"elements":[{"type":"node","id":1,"lat":41.0,"lon":29.0,"tags":{"name":"Cafe"}}]}`))
	}))
	defer server.Close()

	data, err := fetchOne(server.URL, "[out:json];out;", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Elements) != 1 || data.Elements[0].Tags["name"] != "Cafe" {
		t.Errorf("unexpected payload: %+v", data.Elements)
	}
}
