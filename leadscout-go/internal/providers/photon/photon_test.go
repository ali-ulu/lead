package photon

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func withServer(t *testing.T, body string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	prev := endpoint
	endpoint = srv.URL
	t.Cleanup(func() {
		endpoint = prev
		srv.Close()
	})
}

func TestExtentBecomesBBox(t *testing.T) {
	withServer(t, `{"features":[{"properties":{"name":"Berlin","city":"Berlin","country":"Deutschland","countrycode":"DE","extent":[13.088345,52.6755087,13.7611609,52.3382448]},"geometry":{"coordinates":[13.3951309,52.5173885]}}]}`, http.StatusOK)
	area, err := GeocodeArea("Berlin", "Germany", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if area.Lat != 52.5173885 || area.Lon != 13.3951309 {
		t.Errorf("lat/lon = %v/%v", area.Lat, area.Lon)
	}
	if area.BBox.South != 52.3382448 || area.BBox.North != 52.6755087 {
		t.Errorf("bbox = %+v", area.BBox)
	}
	if area.BBox.West != 13.088345 || area.BBox.East != 13.7611609 {
		t.Errorf("bbox = %+v", area.BBox)
	}
	if area.CountryCode != "DE" {
		t.Errorf("country code = %q", area.CountryCode)
	}
}

func TestPointResultDerivesPaddedBBox(t *testing.T) {
	withServer(t, `{"features":[{"properties":{"name":"Somewhere","city":"Somewhere","country":"Testland","countrycode":"tl"},"geometry":{"coordinates":[2.0,1.0]}}]}`, http.StatusOK)
	area, err := GeocodeArea("Somewhere", "Testland", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !(area.BBox.South < area.Lat && area.BBox.North > area.Lat) {
		t.Errorf("latitude not padded: %+v", area.BBox)
	}
	if !(area.BBox.West < area.Lon && area.BBox.East > area.Lon) {
		t.Errorf("longitude not padded: %+v", area.BBox)
	}
}

func TestEmptyFeaturesIsNotFound(t *testing.T) {
	withServer(t, `{"features":[]}`, http.StatusOK)
	if _, err := GeocodeArea("Nowhere", "", 5*time.Second); err == nil {
		t.Fatal("expected an error for empty features")
	}
}

func TestMissingQueryRejected(t *testing.T) {
	if _, err := GeocodeArea("", "", 5*time.Second); err == nil {
		t.Fatal("expected an error for empty query")
	}
}
