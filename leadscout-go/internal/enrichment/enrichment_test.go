package enrichment

import "testing"

// TestHarvestEmailsDropsAssetNamesAndKeepsRealAddress guards the address regex
// that also fires on retina image filenames: "find_provider@2x-480x358.png"
// reads as an address because the extension looks like a TLD.
func TestHarvestEmailsDropsAssetNamesAndKeepsRealAddress(t *testing.T) {
	doc := `<html><body>
<img src="/img/find_provider@2x-480x358.png">
<img src="/img/search_formulary@2x.png">
<a href="mailto:owner@shop.com">mail</a>
<p>Reach us at info@shop.com.tr</p>
</body></html>`
	out := map[string]bool{}
	harvestEmails(doc, []string{"owner@shop.com"}, out)

	if out["find_provider@2x-480x358.png"] || out["search_formulary@2x.png"] {
		t.Errorf("asset filename leaked into emails: %v", out)
	}
	if !out["owner@shop.com"] {
		t.Errorf("mailto address missing: %v", out)
	}
	if !out["info@shop.com.tr"] {
		t.Errorf("inline address missing: %v", out)
	}
}
