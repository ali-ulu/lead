package audit

import (
	"testing"
)

func TestDetectTrackingInlineGtagAndTagManager(t *testing.T) {
	vendors := DetectTracking(
		[]string{"https://www.googletagmanager.com/gtag/js?id=G-XXXX"},
		[]string{"window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}"},
	)
	if !contains(vendors, "google_analytics") {
		t.Errorf("google_analytics not detected: %v", vendors)
	}
}

func TestDetectTrackingMetaPixel(t *testing.T) {
	vendors := DetectTracking(nil, []string{"!function(f,b,e,v,n,t,s){fbq('init','123')}(window);"})
	if !contains(vendors, "meta_pixel") {
		t.Errorf("meta_pixel not detected: %v", vendors)
	}
}

func TestDetectTrackingSortedAndMultiple(t *testing.T) {
	vendors := DetectTracking(
		[]string{"https://connect.facebook.net/en_US/fbevents.js", "https://mc.yandex.ru/metrika/tag.js"},
		[]string{"fbq('track','PageView');"},
	)
	if !contains(vendors, "meta_pixel") || !contains(vendors, "yandex_metrica") {
		t.Errorf("expected meta_pixel and yandex_metrica, got %v", vendors)
	}
	sorted := append([]string{}, vendors...)
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1] > sorted[i] {
			t.Errorf("vendors not sorted: %v", vendors)
		}
	}
}

func TestDetectTrackingCleanPage(t *testing.T) {
	if got := DetectTracking([]string{"/assets/app.js"}, []string{"console.log('hi')"}); len(got) != 0 {
		t.Errorf("expected no vendors, got %v", got)
	}
}

func TestParseSignalsCapturesScripts(t *testing.T) {
	doc := `<html><head>` +
		`<script src="https://example.com/app.js"></script>` +
		`<script>fbq('init','1');</script>` +
		`<script type="application/ld+json">{"@type":"LocalBusiness"}</script>` +
		`</head></html>`
	s := parseSignals(doc)
	if !contains(s.scriptSrcs, "https://example.com/app.js") {
		t.Errorf("script src not captured: %v", s.scriptSrcs)
	}
	foundInline := false
	for _, body := range s.inlineScripts {
		if len(body) > 0 && body[0:4] == "fbq(" {
			foundInline = true
		}
		if body == `{"@type":"LocalBusiness"}` {
			t.Errorf("JSON-LD leaked into inline scripts")
		}
	}
	if !foundInline {
		t.Errorf("inline script body not captured: %v", s.inlineScripts)
	}
	if len(s.jsonLD) != 1 {
		t.Errorf("jsonld blocks = %d, want 1", len(s.jsonLD))
	}
	for _, part := range s.textParts {
		if part == "fbq('init','1');" {
			t.Errorf("inline JS leaked into page text")
		}
	}
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
