package merge

import (
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

func TestDedupesOnNormalizedPhone(t *testing.T) {
	osm := []*model.Lead{{
		Source: "osm", SourceID: "node:1", Name: "Acme Dental", Category: "dentist",
		Latitude: model.Float(41.0), Longitude: model.Float(29.0),
		Phone: model.Str("0555 111 22 33"), SocialLinks: map[string]string{},
		WebsiteStatus: "missing",
	}}
	overture := []*model.Lead{{
		Source: "overture", SourceID: "ov-1", Name: "Acme Dental", Category: "dentist",
		Latitude: model.Float(41.0001), Longitude: model.Float(29.0001),
		Phone: model.Str("+905551112233"), Website: model.Str("https://acme.example"),
		SocialLinks: map[string]string{}, WebsiteStatus: "unknown",
	}}
	rows := MergeLeads(osm, overture)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if _, ok := rows[0].SourceRefs["osm"]; !ok {
		t.Errorf("missing osm ref: %v", rows[0].SourceRefs)
	}
	if _, ok := rows[0].SourceRefs["overture"]; !ok {
		t.Errorf("missing overture ref: %v", rows[0].SourceRefs)
	}
}

func TestPhoneStoredAsE164(t *testing.T) {
	rows := MergeLeads([]*model.Lead{{
		Source: "osm", SourceID: "n:1", Name: "Kuafor A", Category: "barber",
		Latitude: model.Float(1), Longitude: model.Float(2),
		Phone: model.Str("0555 999 88 77"), SocialLinks: map[string]string{},
	}})
	if got := *rows[0].Phone; got != "+905559998877" {
		t.Errorf("got %q", got)
	}
}

func TestWebsiteStoredNormalized(t *testing.T) {
	rows := MergeLeads([]*model.Lead{{
		Source: "osm", SourceID: "n:2", Name: "Bella", Category: "restaurant",
		Latitude: model.Float(1), Longitude: model.Float(2),
		Website:     model.Str("http://www.bella.example/?utm_source=x"),
		SocialLinks: map[string]string{}, WebsiteStatus: "missing",
	}})
	if got := *rows[0].Website; got != "http://bella.example" {
		t.Errorf("website = %q", got)
	}
	if rows[0].WebsiteStatus != "unknown" {
		t.Errorf("status = %q, want unknown", rows[0].WebsiteStatus)
	}
}

func TestFuzzyNameMergeRequiresProximity(t *testing.T) {
	near := []*model.Lead{
		{Source: "osm", SourceID: "a", Name: "Sarıtaş Döner", Category: "restaurant", Latitude: model.Float(38), Longitude: model.Float(30), SocialLinks: map[string]string{}},
		{Source: "overture", SourceID: "b", Name: "Sarıtaş Döner Salonu", Category: "restaurant", Latitude: model.Float(38.0001), Longitude: model.Float(30.0001), SocialLinks: map[string]string{}},
	}
	if len(MergeLeads(near)) != 1 {
		t.Errorf("near rows should merge")
	}
	far := []*model.Lead{
		{Source: "osm", SourceID: "a", Name: "Sarıtaş Döner", Category: "restaurant", Latitude: model.Float(38), Longitude: model.Float(30), SocialLinks: map[string]string{}},
		{Source: "overture", SourceID: "b", Name: "Sarıtaş Döner Salonu", Category: "restaurant", Latitude: model.Float(39.5), Longitude: model.Float(32), SocialLinks: map[string]string{}},
	}
	if len(MergeLeads(far)) != 2 {
		t.Errorf("far rows should not merge")
	}
}

func TestDifferentBusinessesStaySeparate(t *testing.T) {
	rows := MergeLeads([]*model.Lead{
		{Source: "osm", SourceID: "a", Name: "Acme Dental", Category: "dentist", Latitude: model.Float(41), Longitude: model.Float(29), SocialLinks: map[string]string{}},
		{Source: "osm", SourceID: "b", Name: "Bella Pizza", Category: "restaurant", Latitude: model.Float(41), Longitude: model.Float(29), SocialLinks: map[string]string{}},
	})
	if len(rows) != 2 {
		t.Errorf("got %d rows, want 2", len(rows))
	}
}

func TestCategoryCanonicalized(t *testing.T) {
	rows := MergeLeads([]*model.Lead{{
		Source: "osm", SourceID: "a", Name: "Berber Ali", Category: "hairdresser",
		Latitude: model.Float(1), Longitude: model.Float(2), SocialLinks: map[string]string{},
	}})
	if rows[0].Category != "barber" {
		t.Errorf("got %q", rows[0].Category)
	}
}

func TestWebsiteMatchMergesAcrossDistance(t *testing.T) {
	rows := MergeLeads(
		[]*model.Lead{{Source: "osm", SourceID: "a", Name: "Alpha", Category: "hotel", Latitude: model.Float(1), Longitude: model.Float(2), Website: model.Str("https://www.alpha.example"), SocialLinks: map[string]string{}}},
		[]*model.Lead{{Source: "overture", SourceID: "b", Name: "Alpha Hotel", Category: "hotel", Latitude: model.Float(5), Longitude: model.Float(6), Website: model.Str("http://alpha.example/"), SocialLinks: map[string]string{}}},
	)
	if len(rows) != 1 {
		t.Errorf("got %d rows, want 1", len(rows))
	}
}
