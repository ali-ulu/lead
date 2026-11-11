package netguard

import (
	"errors"
	"net"
	"testing"
	"time"
)

func withLookup(t *testing.T, ips []string, err error) {
	t.Helper()
	original := lookupIP
	lookupIP = func(host string) ([]net.IP, error) {
		if err != nil {
			return nil, err
		}
		out := make([]net.IP, len(ips))
		for i, s := range ips {
			out[i] = net.ParseIP(s)
		}
		return out, nil
	}
	t.Cleanup(func() { lookupIP = original })
}

func TestPublicHostAllowed(t *testing.T) {
	withLookup(t, []string{"93.184.216.34"}, nil)
	if err := AssertPublicURL("https://example.com/"); err != nil {
		t.Errorf("public host rejected: %v", err)
	}
}

func TestLoopbackAndPrivateBlocked(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.5", "192.168.1.1", "172.16.0.1", "169.254.169.254", "0.0.0.0"} {
		withLookup(t, []string{ip}, nil)
		if err := AssertPublicURL("http://internal.example/"); !errors.Is(err, ErrPrivate) {
			t.Errorf("ip %s not blocked: %v", ip, err)
		}
	}
}

func TestLocalhostNameBlockedWithoutDNS(t *testing.T) {
	if err := AssertPublicURL("http://localhost:8000/admin"); !errors.Is(err, ErrPrivate) {
		t.Errorf("localhost not blocked: %v", err)
	}
}

func TestUnresolvableReported(t *testing.T) {
	withLookup(t, nil, &net.DNSError{Err: "no such host", Name: "retired.example"})
	err := AssertPublicURL("http://retired.example/")
	if !errors.Is(err, ErrUnresolved) || !IsUnresolved(err) {
		t.Errorf("expected unresolved error, got %v", err)
	}
}

func TestParseRobotsSpecificAgentRules(t *testing.T) {
	body := "User-agent: *\nDisallow: /all\nUser-agent: LeadScout\nDisallow: /private\nDisallow: /tmp*\n"
	rules := parseRobots(body)
	if len(rules) != 2 || rules[0] != "/private" || rules[1] != "/tmp*" {
		t.Errorf("rules = %v", rules)
	}
}

func TestParseRobotsWildcardGroup(t *testing.T) {
	rules := parseRobots("User-agent: *\nDisallow: /secret\nAllow: /\n")
	if len(rules) != 1 || rules[0] != "/secret" {
		t.Errorf("rules = %v", rules)
	}
}

func TestRateLimiterSpacesCalls(t *testing.T) {
	limiter := NewLimiter(50 * time.Millisecond)
	start := time.Now()
	limiter.Wait()
	limiter.Wait()
	if time.Since(start) < 40*time.Millisecond {
		t.Errorf("calls not spaced")
	}
}

func TestRateLimiterDisabledIsNoop(t *testing.T) {
	limiter := NewLimiter(0)
	start := time.Now()
	for i := 0; i < 5; i++ {
		limiter.Wait()
	}
	if time.Since(start) > 20*time.Millisecond {
		t.Errorf("disabled limiter blocked")
	}
}
