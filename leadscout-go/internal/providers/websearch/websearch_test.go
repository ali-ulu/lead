package websearch

import (
	"os"
	"strings"
	"testing"
)

var searchEnvKeys = []string{"LEADSCOUT_WEB_SEARCH", "BRAVE_SEARCH_API_KEY", "SEARXNG_URL", "GOOGLE_CSE_API_KEY", "GOOGLE_CSE_ID"}

func clearSearchEnv(t *testing.T) {
	t.Helper()
	saved := map[string]*string{}
	for _, k := range searchEnvKeys {
		if v, ok := os.LookupEnv(k); ok {
			saved[k] = &v
			os.Unsetenv(k)
		}
	}
	t.Cleanup(func() {
		for _, k := range searchEnvKeys {
			os.Unsetenv(k)
		}
		for k, v := range saved {
			os.Setenv(k, *v)
		}
	})
}

func TestDefaultChainIsKeylessThenGoogle(t *testing.T) {
	clearSearchEnv(t)
	if got := ConfiguredProviders(); len(got) != 1 || got[0] != "duckduckgo" {
		t.Errorf("chain = %v", got)
	}
	os.Setenv("GOOGLE_CSE_API_KEY", "key")
	os.Setenv("GOOGLE_CSE_ID", "cx")
	got := ConfiguredProviders()
	if len(got) != 2 || got[0] != "duckduckgo" || got[1] != "google" {
		t.Errorf("chain = %v", got)
	}
}

func TestGoogleNeedsBothKeyAndCX(t *testing.T) {
	clearSearchEnv(t)
	os.Setenv("GOOGLE_CSE_API_KEY", "key")
	for _, p := range ConfiguredProviders() {
		if p == "google" {
			t.Errorf("google should not be configured with key only")
		}
	}
}

func TestExplicitChainAndOff(t *testing.T) {
	clearSearchEnv(t)
	os.Setenv("GOOGLE_CSE_API_KEY", "key")
	os.Setenv("GOOGLE_CSE_ID", "cx")
	os.Setenv("SEARXNG_URL", "http://127.0.0.1:8080")
	os.Setenv("LEADSCOUT_WEB_SEARCH", "searxng, google , duckduckgo")
	got := ConfiguredProviders()
	if strings.Join(got, ",") != "searxng,google,duckduckgo" {
		t.Errorf("chain = %v", got)
	}
	os.Setenv("LEADSCOUT_WEB_SEARCH", "brave")
	if len(ConfiguredProviders()) != 0 {
		t.Errorf("brave without key should be empty")
	}
	os.Setenv("LEADSCOUT_WEB_SEARCH", "off")
	if len(ConfiguredProviders()) != 0 {
		t.Errorf("off should disable search")
	}
}

const ddgHTML = `
<div class="result">
  <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample-dental.pk%2F">Example Dental</a>
  <a class="result__snippet">Example Dental in Karachi, official website</a>
</div>
<div class="result">
  <a class="result__a" href="https://instagram.com/exampledental">Example Dental (@exampledental)</a>
</div>
`

func TestDuckDuckGoResultsParsedAndUnwrapped(t *testing.T) {
	results, err := parseDuckDuckGo(ddgHTML, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d", len(results))
	}
	if results[0].URL != "https://example-dental.pk/" {
		t.Errorf("url not unwrapped: %s", results[0].URL)
	}
}

func TestDuckDuckGoBotCheckRaises(t *testing.T) {
	_, err := parseDuckDuckGo("<html>unusual traffic detected</html>", 10)
	if err == nil {
		t.Errorf("bot-check page should error")
	}
}

func TestDirectoryHostsAreNotOfficial(t *testing.T) {
	for _, host := range []string{"https://instagram.com/example", "https://yelp.com/biz/x", "https://www.facebook.com/x"} {
		if !isDirectory(host) {
			t.Errorf("%s should be a directory/social host", host)
		}
	}
	if isDirectory("https://example-dental.pk/") {
		t.Errorf("official site misclassified as directory")
	}
}

func TestScoreResultFavorsOfficialSite(t *testing.T) {
	official := scoreResult(Result{URL: "https://example-dental.pk/", Title: "Example Dental", Description: "Karachi dentist official website"}, "Example Dental", "Karachi", "Pakistan")
	directory := scoreResult(Result{URL: "https://yelp.com/biz/example-dental", Title: "Example Dental - Yelp", Description: "Reviews"}, "Example Dental", "Karachi", "Pakistan")
	if directory != 0 {
		t.Errorf("directory should score 0, got %v", directory)
	}
	if official < 0.58 {
		t.Errorf("official score too low: %v", official)
	}
}
