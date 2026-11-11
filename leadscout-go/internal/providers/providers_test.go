package providers

import (
	"strings"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
	"github.com/ali-ulu/lead/leadscout-go/internal/normalize"
	"github.com/ali-ulu/lead/leadscout-go/internal/outreach"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/osm"
)

func TestAroundQueryHasNoOutputLimit(t *testing.T) {
	q, err := osm.BuildAroundQuery(24.8607, 67.0011, 20000, "dentist", 35)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "amenity") {
		t.Errorf("query missing amenity: %s", q)
	}
	if !strings.Contains(q, "around:20000,24.8607,67.0011") {
		t.Errorf("query missing around clause: %s", q)
	}
	if !strings.Contains(q, "out tags center qt;") {
		t.Errorf("query missing out clause: %s", q)
	}
	if strings.Contains(q, " 250;") {
		t.Errorf("query still limited to 250: %s", q)
	}
}

func TestNewSectorCategoriesSupported(t *testing.T) {
	for _, category := range []string{"construction", "manufacturer"} {
		q, err := osm.BuildAroundQuery(38.42, 27.13, 20000, category, 35)
		if err != nil || q == "" {
			t.Errorf("category %s not supported: %v", category, err)
		}
	}
}

func TestNormalizeUnlimitedMode(t *testing.T) {
	elements := make([]map[string]any, 600)
	for i := range elements {
		elements[i] = map[string]any{"type": "node", "id": i, "lat": 1.0, "lon": 2.0, "tags": map[string]any{"name": "Business"}}
	}
	rows := osm.NormalizeFromElements(elements, "restaurant", "Test City", "Test Country", 0)
	if len(rows) != 600 {
		t.Errorf("got %d rows, want 600", len(rows))
	}
}

func TestSocialHandleBecomesURL(t *testing.T) {
	got := osm.Social(map[string]any{"contact:instagram": "@example"})
	if got != "https://instagram.com/example" {
		t.Errorf("got %q", got)
	}
}

func TestMultipleSocialChannels(t *testing.T) {
	socials := osm.Socials(map[string]any{
		"contact:instagram": "@leadscout",
		"contact:facebook":  "leadscout.example",
		"contact:whatsapp":  "+92 300 1234567",
	})
	if socials["instagram"] != "https://instagram.com/leadscout" {
		t.Errorf("instagram = %q", socials["instagram"])
	}
	if socials["facebook"] != "https://facebook.com/leadscout.example" {
		t.Errorf("facebook = %q", socials["facebook"])
	}
	if socials["whatsapp"] != "https://wa.me/923001234567" {
		t.Errorf("whatsapp = %q", socials["whatsapp"])
	}
}

func TestUrduOutreach(t *testing.T) {
	msg := outreach.BuildMessage(&model.Lead{Name: "Example", WebsiteStatus: "missing"}, "ur")
	if !strings.Contains(msg, "السلام علیکم") {
		t.Errorf("urdu greeting missing")
	}
}

func TestSindhiOutreach(t *testing.T) {
	msg := outreach.BuildMessage(&model.Lead{Name: "Example", WebsiteStatus: "missing"}, "sd")
	if !strings.Contains(msg, "السلام عليڪم") {
		t.Errorf("sindhi greeting missing")
	}
}

func TestTargetSectorsAreDiscoverable(t *testing.T) {
	for _, key := range []string{"construction", "pest_control", "manufacturer", "guest_house"} {
		if _, ok := osm.CategoryFilters[key]; !ok {
			t.Errorf("category %s missing", key)
		}
	}
	if got := normalize.CategoryKey("mantolama"); got != "construction" {
		t.Errorf("mantolama -> %q", got)
	}
	if got := normalize.CategoryKey("pansiyon"); got != "guest_house" {
		t.Errorf("pansiyon -> %q", got)
	}
}
