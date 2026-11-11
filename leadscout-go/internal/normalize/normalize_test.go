package normalize

import "testing"

func TestTurkishPhoneFormsConverge(t *testing.T) {
	const expected = "+905551112233"
	for _, raw := range []string{
		"+90 555 111 22 33", "0555 111 22 33", "05551112233",
		"555 111 22 33", "(0555) 111-22-33", "0090 555 111 22 33",
	} {
		if got := NormalizePhone(raw, "TR"); got != expected {
			t.Errorf("NormalizePhone(%q) = %q, want %q", raw, got, expected)
		}
	}
}

func TestPhoneKeyDropsPrefixes(t *testing.T) {
	for _, raw := range []string{"+90 555 111 22 33", "0555 111 22 33", "5551112233"} {
		if got := PhoneKey(raw, "TR"); got != "5551112233" {
			t.Errorf("PhoneKey(%q) = %q, want 5551112233", raw, got)
		}
	}
}

func TestNonTurkishKeepsDigits(t *testing.T) {
	if got := NormalizePhone("030 1234567", "DE"); got != "0301234567" {
		t.Errorf("got %q", got)
	}
}

func TestInternationalPrefix(t *testing.T) {
	if got := NormalizePhone("+1 (202) 555-0143", "US"); got != "+12025550143" {
		t.Errorf("got %q", got)
	}
	if got := NormalizePhone("0012025550143", "US"); got != "+12025550143" {
		t.Errorf("got %q", got)
	}
}

func TestEmptyAndLettersReturnEmpty(t *testing.T) {
	for _, raw := range []string{"", "no phone"} {
		if got := NormalizePhone(raw, "TR"); got != "" {
			t.Errorf("NormalizePhone(%q) = %q, want empty", raw, got)
		}
	}
}

func TestWebsiteNormalization(t *testing.T) {
	cases := map[string]string{
		"http://www.Example.com/":                                  "http://example.com",
		"https://Example.com/path/?utm_source=x&utm_medium=y#frag": "https://example.com/path",
		"example.com/page?id=7&utm_campaign=spring":                "https://example.com/page?id=7",
		"acme.example": "https://acme.example",
	}
	for in, want := range cases {
		if got := NormalizeWebsite(in); got != want {
			t.Errorf("NormalizeWebsite(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDomainHelper(t *testing.T) {
	if got := NormalizeDomain("www.Acme.example/path"); got != "acme.example" {
		t.Errorf("got %q", got)
	}
	if got := NormalizeDomain("https://acme.example"); got != "acme.example" {
		t.Errorf("got %q", got)
	}
	if got := NormalizeDomain(""); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestNameFoldsTurkishDiacritics(t *testing.T) {
	if got := NormalizeName("Çiçekçi Şükrü"); got != "cicekcisukru" {
		t.Errorf("got %q", got)
	}
}

func TestSimilarityMatchesNearNames(t *testing.T) {
	if got := NameSimilarity("Acme Dental", "Acme Dent"); got < 0.86 {
		t.Errorf("got %v", got)
	}
	if got := NameSimilarity("Sarıtaş Döner", "Sarıtaş Döner Salonu"); got < 0.86 {
		t.Errorf("got %v", got)
	}
}

func TestSimilarityRejectsDifferentNames(t *testing.T) {
	if got := NameSimilarity("Acme Dental", "Bella Pizza"); got >= 0.86 {
		t.Errorf("got %v", got)
	}
}

func TestCategoryAliases(t *testing.T) {
	cases := map[string]string{
		"hairdresser": "barber", "Berber": "barber", "kuaför": "barber",
		"Dişçi": "dentist", "eczane": "pharmacy", "mantolama": "construction",
		"pansiyon": "guest_house",
	}
	for in, want := range cases {
		if got := CategoryKey(in); got != want {
			t.Errorf("CategoryKey(%q) = %q, want %q", in, got, want)
		}
	}
	if got := CategoryKey("Pet Shop"); got != "petshop" {
		t.Errorf("got %q", got)
	}
}
