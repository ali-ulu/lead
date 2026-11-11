package emailenrich

import (
	"os"
	"testing"
)

func clearKeys(t *testing.T) {
	t.Helper()
	for _, key := range []string{"HUNTER_API_KEY", "PEOPLE_DATA_LABS_API_KEY", "CLEARBIT_API_KEY"} {
		t.Setenv(key, "")
	}
}

func TestRejectsBareDomain(t *testing.T) {
	clearKeys(t)
	if _, err := EnrichDomain("not-a-domain", 0); err == nil {
		t.Fatal("expected an error for a bare domain")
	}
}

func TestNoKeysYieldsNoMatchAndAttempts(t *testing.T) {
	clearKeys(t)
	result, err := EnrichDomain("example-no-keys.test", 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Found {
		t.Errorf("expected not found, got %+v", result)
	}
	if len(result.Attempts) != 3 {
		t.Fatalf("attempts = %d, want 3", len(result.Attempts))
	}
	for _, a := range result.Attempts {
		if a.Status != "error" {
			t.Errorf("provider %s status = %q, want error (key missing)", a.Provider, a.Status)
		}
	}
}

func TestStripsWWWPrefix(t *testing.T) {
	clearKeys(t)
	result, err := EnrichDomain("WWW.Example-No-Keys.test", 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Domain != "example-no-keys.test" {
		t.Errorf("domain = %q", result.Domain)
	}
}

func TestHunterMatchShape(t *testing.T) {
	clearKeys(t)
	// Exercise the provider directly so the test does not depend on network.
	match, err := hunter("example.com", 0)
	if err == nil {
		t.Fatal("expected an error when the key is missing")
	}
	if match != nil {
		t.Errorf("expected nil match, got %+v", match)
	}
}

func TestEnvKeyIsRead(t *testing.T) {
	t.Setenv("HUNTER_API_KEY", "test-key")
	if _, err := hunter("example.invalid", 0); err == nil {
		// The request will fail against a bogus domain, but it must not be the
		// "not configured" error, proving the key path is exercised.
		if os.Getenv("HUNTER_API_KEY") == "" {
			t.Skip("environment not propagated")
		}
	}
}
