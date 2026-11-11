// Package cache is a small JSON response cache for public data providers.
// It is disabled unless LEADSCOUT_CACHE_TTL is a positive number of seconds.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func cacheDir() string {
	if configured := os.Getenv("LEADSCOUT_CACHE_DIR"); configured != "" {
		return configured
	}
	return filepath.Join(os.TempDir(), "leadscout-cache")
}

// TTLSeconds returns the configured cache TTL, 0 when disabled.
func TTLSeconds() int {
	raw := os.Getenv("LEADSCOUT_CACHE_TTL")
	if raw == "" {
		return 0
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	v := int(f)
	if v < 0 {
		return 0
	}
	return v
}

// Enabled reports whether the cache is active.
func Enabled() bool { return TTLSeconds() > 0 }

func key(kind string, parts any) string {
	blob, _ := json.Marshal([]any{kind, parts})
	sum := sha256.Sum256(blob)
	return kind + "-" + hex.EncodeToString(sum[:])[:24]
}

func path(k string) string { return filepath.Join(cacheDir(), k+".json") }

// Get returns the cached value decoded into out, or false when missing/expired.
func Get(kind string, parts any, out any) bool {
	if !Enabled() {
		return false
	}
	p := path(key(kind, parts))
	stat, err := os.Stat(p)
	if err != nil {
		return false
	}
	if time.Since(stat.ModTime()) > time.Duration(TTLSeconds())*time.Second {
		return false
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

// Put stores a value in the cache (best effort).
func Put(kind string, parts any, value any) {
	if !Enabled() {
		return
	}
	if err := os.MkdirAll(cacheDir(), 0o755); err != nil {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	p := path(key(kind, parts))
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, p)
}

// GetOrCall returns the cached value or calls produce once and caches it.
func GetOrCall[T any](kind string, parts any, produce func() (T, error)) (T, error) {
	var cached T
	if Get(kind, parts, &cached) {
		return cached, nil
	}
	value, err := produce()
	if err != nil {
		var zero T
		return zero, err
	}
	Put(kind, parts, value)
	return value, nil
}
