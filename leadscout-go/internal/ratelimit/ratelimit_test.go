package ratelimit

import (
	"errors"
	"testing"
	"time"
)

func TestReturnsValueAndRetriesTransient(t *testing.T) {
	Reset()
	attempts := 0
	value, err := Call("p", func() (string, error) {
		attempts++
		if attempts < 3 {
			return "", &timeoutErr{}
		}
		return "ok", nil
	}, Options{Retries: 3, Backoff: 0})
	if err != nil || value != "ok" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestNonTransientIsNotRetried(t *testing.T) {
	Reset()
	attempts := 0
	_, err := Call("p", func() (string, error) {
		attempts++
		return "", errors.New("bot-check")
	}, Options{Retries: 3, Backoff: 0})
	if err == nil {
		t.Fatalf("expected error")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestCircuitOpensAfterRepeatedFailures(t *testing.T) {
	Reset()
	Configure("p", 0, 2, 60)
	broken := func() (string, error) { return "", errors.New("down") }
	for i := 0; i < 2; i++ {
		if _, err := Call("p", broken, Options{Retries: 0}); err == nil {
			t.Fatalf("expected failure")
		}
	}
	if _, err := Call("p", func() (string, error) { return "ok", nil }, Options{Retries: 0}); !errors.Is(err, CircuitOpen) {
		t.Errorf("expected CircuitOpen, got %v", err)
	}
}

func TestSuccessResetsCircuit(t *testing.T) {
	Reset()
	Configure("p", 0, 2, 60)
	broken := func() (string, error) { return "", errors.New("down") }
	Call("p", broken, Options{Retries: 0})
	if _, err := Call("p", func() (string, error) { return "ok", nil }, Options{Retries: 0}); err != nil {
		t.Fatalf("expected success")
	}
	Call("p", broken, Options{Retries: 0})
	// Only one failure since the reset, so the breaker is still closed.
	if _, err := Call("p", func() (string, error) { return "ok", nil }, Options{Retries: 0}); err != nil {
		t.Errorf("breaker should still be closed: %v", err)
	}
}

func TestMinIntervalSpacesCalls(t *testing.T) {
	Reset()
	start := time.Now()
	Call("p", func() (int, error) { return 1, nil }, Options{MinInterval: 0.05})
	Call("p", func() (int, error) { return 2, nil }, Options{MinInterval: 0.05})
	if time.Since(start) < 50*time.Millisecond {
		t.Errorf("calls not spaced")
	}
}

type timeoutErr struct{}

func (e *timeoutErr) Error() string   { return "timeout" }
func (e *timeoutErr) Timeout() bool   { return true }
func (e *timeoutErr) Temporary() bool { return true }
