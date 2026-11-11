package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func withCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.Setenv("LEADSCOUT_CACHE_DIR", dir)
	os.Setenv("LEADSCOUT_CACHE_TTL", "3600")
	t.Cleanup(func() {
		os.Unsetenv("LEADSCOUT_CACHE_DIR")
		os.Unsetenv("LEADSCOUT_CACHE_TTL")
	})
	return dir
}

func TestDisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("LEADSCOUT_CACHE_DIR", dir)
	os.Unsetenv("LEADSCOUT_CACHE_TTL")
	defer os.Unsetenv("LEADSCOUT_CACHE_DIR")
	if Enabled() {
		t.Errorf("cache should be disabled without TTL")
	}
	Put("k", []any{"a"}, map[string]any{"v": 1})
	var out map[string]any
	if Get("k", []any{"a"}, &out) {
		t.Errorf("disabled cache returned a value")
	}
}

func TestRoundTripAndKeyIsolation(t *testing.T) {
	withCacheDir(t)
	Put("overpass", []any{"q1"}, map[string]any{"elements": []any{1}})
	var got map[string]any
	if !Get("overpass", []any{"q1"}, &got) {
		t.Fatalf("expected hit")
	}
	if !Get("overpass", []any{"q1"}, &got) {
		t.Fatalf("expected hit")
	}
	var miss map[string]any
	if Get("overpass", []any{"q2"}, &miss) {
		t.Errorf("wrong key hit")
	}
	if Get("nominatim", []any{"q1"}, &miss) {
		t.Errorf("wrong kind hit")
	}
}

func TestExpiredEntryIgnored(t *testing.T) {
	dir := withCacheDir(t)
	Put("k", []any{"a"}, map[string]any{"v": 1})
	matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(matches) == 0 {
		t.Fatalf("no cache file written")
	}
	old := time.Now().Add(-10 * time.Hour)
	os.Chtimes(matches[0], old, old)
	var out map[string]any
	if Get("k", []any{"a"}, &out) {
		t.Errorf("expired entry returned")
	}
}

func TestGetOrCallProducesOnce(t *testing.T) {
	withCacheDir(t)
	calls := 0
	produce := func() (map[string]any, error) {
		calls++
		return map[string]any{"n": calls}, nil
	}
	first, _ := GetOrCall("produce", []any{"a"}, produce)
	second, _ := GetOrCall("produce", []any{"a"}, produce)
	if fmt.Sprint(first["n"]) != "1" || fmt.Sprint(second["n"]) != "1" {
		t.Errorf("produce ran more than once: %v %v", first, second)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}
