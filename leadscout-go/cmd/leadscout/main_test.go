package main

import (
	"os"
	"testing"
)

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "x", "y"); got != "x" {
		t.Errorf("got %q, want x", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestParseOrigins(t *testing.T) {
	got := parseOrigins("http://a.example, https://b.example ,, ")
	if len(got) != 2 || !got["http://a.example"] || !got["https://b.example"] {
		t.Errorf("origins = %v", got)
	}
	if len(parseOrigins("")) != 0 {
		t.Errorf("empty string should yield no origins")
	}
}

func TestEnvInt(t *testing.T) {
	os.Setenv("LEADSCOUT_TEST_INT", "42")
	defer os.Unsetenv("LEADSCOUT_TEST_INT")
	if got := envInt("LEADSCOUT_TEST_INT", 7); got != 42 {
		t.Errorf("got %d, want 42", got)
	}
	os.Setenv("LEADSCOUT_TEST_INT", "not-a-number")
	if got := envInt("LEADSCOUT_TEST_INT", 7); got != 7 {
		t.Errorf("invalid value should fall back to default, got %d", got)
	}
	if got := envInt("LEADSCOUT_UNSET_INT", 9); got != 9 {
		t.Errorf("unset should use default, got %d", got)
	}
}

func TestEnvInt64(t *testing.T) {
	os.Setenv("LEADSCOUT_TEST_INT64", "123456789012")
	defer os.Unsetenv("LEADSCOUT_TEST_INT64")
	if got := envInt64("LEADSCOUT_TEST_INT64", 1); got != 123456789012 {
		t.Errorf("got %d", got)
	}
	if got := envInt64("LEADSCOUT_UNSET_INT64", 5); got != 5 {
		t.Errorf("unset should use default, got %d", got)
	}
}
