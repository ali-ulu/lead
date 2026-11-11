package audit

import (
	"testing"
)

func TestSpeedProxyMonotonicAndContinuous(t *testing.T) {
	ms := []int{100, 200, 700, 750, 1200, 1250, 2500, 5000}
	prev := 1000
	for _, v := range ms {
		got := SpeedProxy(v)
		if got > prev {
			t.Errorf("SpeedProxy not monotonic at %d: %d > %d", v, got, prev)
		}
		prev = got
	}
	if SpeedProxy(700) == SpeedProxy(1199) {
		t.Errorf("bucket collision: 700 and 1199 produce the same value")
	}
}

func TestSpeedProxyBounds(t *testing.T) {
	if SpeedProxy(0) > 100 {
		t.Errorf("SpeedProxy(0) = %d, want <= 100", SpeedProxy(0))
	}
	if SpeedProxy(60000) < 5 {
		t.Errorf("SpeedProxy(60000) = %d, want >= 5", SpeedProxy(60000))
	}
}

func TestNormalizeURL(t *testing.T) {
	got, err := NormalizeURL("example.com")
	if err != nil || got != "https://example.com" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := NormalizeURL("ftp://example.com"); err == nil {
		t.Errorf("ftp should be rejected")
	}
	if _, err := NormalizeURL(""); err == nil {
		t.Errorf("empty should be rejected")
	}
}

func TestStructuredDataExtraction(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	{"@context":"https://schema.org","@type":"Dentist","name":"Example Dental",
	 "address":{"@type":"PostalAddress","addressLocality":"Karachi"},
	 "telephone":"+92 21 0000000","url":"https://example.test","sameAs":["https://instagram.com/example"]}
	</script></head><body><main><h1>Example Dental</h1><h2>How can we help?</h2></main></body></html>`
	s := parseSignals(html)
	types, fields := structuredData(s.jsonLD)
	if !containsStr(types, "Dentist") {
		t.Errorf("Dentist type missing: %v", types)
	}
	if !containsStr(fields, "address") || !containsStr(fields, "sameAs") {
		t.Errorf("fields missing: %v", fields)
	}
	if !s.semanticMain {
		t.Errorf("semantic main not detected")
	}
	if !containsStr(s.headingTexts, "How can we help?") {
		t.Errorf("heading text missing: %v", s.headingTexts)
	}
}

func TestSocialLinksFromMarkup(t *testing.T) {
	html := `<a href="https://www.instagram.com/example">Instagram</a><a href="https://www.facebook.com/example">Facebook</a>`
	s := parseSignals(html)
	if _, ok := s.socialLinks["instagram"]; !ok {
		t.Errorf("instagram not found: %v", s.socialLinks)
	}
	if _, ok := s.socialLinks["facebook"]; !ok {
		t.Errorf("facebook not found: %v", s.socialLinks)
	}
}

func TestDeadDomainReportedAsDead(t *testing.T) {
	// .invalid never resolves, so this exercises the real DNS-failure path.
	result, err := AuditURL("http://retired-domain.invalid/", 0)
	if err != nil {
		t.Fatalf("dead domain should not error: %v", err)
	}
	if result.Reachable {
		t.Errorf("reachable = true")
	}
	if result.WebsiteStatus != "dead" {
		t.Errorf("website_status = %q, want dead", result.WebsiteStatus)
	}
	if result.Error == "" {
		t.Errorf("expected an error message")
	}
	if len(result.SocialLinks) != 0 {
		t.Errorf("social_links = %v, want empty", result.SocialLinks)
	}
}

func TestPrivateHostStillErrors(t *testing.T) {
	if _, err := AuditURL("http://127.0.0.1/", 0); err == nil {
		t.Errorf("private host should error")
	}
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
