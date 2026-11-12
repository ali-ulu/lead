package emailverify

import "testing"

func TestMalformedAddressIsInvalid(t *testing.T) {
	r := VerifyEmail("not-an-email", false, 0)
	if r.ValidSyntax || r.Status != "invalid" || r.Score != 0 {
		t.Errorf("unexpected result: %+v", r)
	}
}

func TestBusinessAddressIsValid(t *testing.T) {
	r := VerifyEmail("owner@localplumber.co.uk", false, 0)
	if !r.ValidSyntax || r.Status != "valid" || r.RoleAccount {
		t.Errorf("unexpected result: %+v", r)
	}
}

func TestRoleAccountIsPenalised(t *testing.T) {
	r := VerifyEmail("info@localplumber.co.uk", false, 0)
	if !r.RoleAccount || r.Score >= 100 {
		t.Errorf("unexpected result: %+v", r)
	}
}

func TestDisposableDomainIsInvalid(t *testing.T) {
	r := VerifyEmail("someone@mailinator.com", false, 0)
	if !r.Disposable || r.Status != "invalid" {
		t.Errorf("unexpected result: %+v", r)
	}
}

func TestFreeProviderFlagged(t *testing.T) {
	r := VerifyEmail("someone@gmail.com", false, 0)
	if !r.FreeProvider || r.Status != "valid" {
		t.Errorf("unexpected result: %+v", r)
	}
}

func TestLowercasesAndTrims(t *testing.T) {
	r := VerifyEmail("  OWNER@Shop.COM ", false, 0)
	if r.Email != "owner@shop.com" {
		t.Errorf("email = %q", r.Email)
	}
}

func TestMXLookupPathSetsFlag(t *testing.T) {
	// example.com publishes no MX record; the lookup must not panic and must
	// set MXFound to a non-nil value when checking is enabled.
	r := VerifyEmail("nobody@example.com", true, 0)
	if r.MXFound == nil {
		t.Fatal("MXFound should be set when checkMX is true")
	}
}

func TestAssetFilenameIsNotAnAddress(t *testing.T) {
	// A retina image name matches the address regex because the extension reads
	// as a TLD; it must be rejected rather than scored as a valid mailbox.
	r := VerifyEmail("find_provider@2x-480x358.png", false, 0)
	if r.ValidSyntax || r.Status != "invalid" || r.Score != 0 {
		t.Errorf("asset filename should be invalid, got %+v", r)
	}
}

func TestLooksLikeEmailFiltersAssetsKeepsReal(t *testing.T) {
	good := []string{"owner@shop.com", "info@firma.com.tr", "a.b+c@mail.example.org"}
	for _, e := range good {
		if !LooksLikeEmail(e) {
			t.Errorf("LooksLikeEmail(%q) = false, want true", e)
		}
	}
	bad := []string{"sprite@2x.png", "icon@3x-480x358.jpg", "logo@2x.svg", "bundle@app.js", "font@2x.woff2"}
	for _, e := range bad {
		if LooksLikeEmail(e) {
			t.Errorf("LooksLikeEmail(%q) = true, want false", e)
		}
	}
}
