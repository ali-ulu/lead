package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func testServer(host, token string) *Server {
	return New(Config{Host: host, Port: 8787, APIToken: token, MaxBody: 1024, RatePerMin: 0})
}

func TestTokenRequiredWhenConfigured(t *testing.T) {
	s := testServer("127.0.0.1", "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/leads", nil)
	if s.tokenOK(req) {
		t.Errorf("missing token should fail")
	}
	req.Header.Set("Authorization", "Bearer secret")
	if !s.tokenOK(req) {
		t.Errorf("valid token should pass")
	}
}

func TestHealthIsPublic(t *testing.T) {
	s := testServer("0.0.0.0", "secret")
	if !s.authorized("/api/v1/health") {
		t.Errorf("health should be public")
	}
}

func TestPublicBindWithoutTokenClosesAPI(t *testing.T) {
	s := testServer("0.0.0.0", "")
	if s.authorized("/api/v1/leads") {
		t.Errorf("public bind without token must close the API")
	}
	if s.authorized("/api/leads") || s.authorized("/api/clear") {
		t.Errorf("legacy routes must also be closed")
	}
	if !s.authorized("/api/health") {
		t.Errorf("health stays public")
	}
}

func TestLoopbackWithoutTokenAllows(t *testing.T) {
	s := testServer("127.0.0.1", "")
	if !s.authorized("/api/v1/leads") {
		t.Errorf("loopback without token should allow")
	}
}

func TestStaticIsAlwaysPublic(t *testing.T) {
	s := testServer("0.0.0.0", "secret")
	if !s.authorized("/index.html") {
		t.Errorf("static must be public")
	}
}

func TestCrossOriginRejected(t *testing.T) {
	s := testServer("127.0.0.1", "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/leads", nil)
	req.Header.Set("Origin", "https://evil.example")
	if s.originAllowed(req) {
		t.Errorf("cross origin should be rejected")
	}
}

func TestSameOriginAllowed(t *testing.T) {
	s := testServer("127.0.0.1", "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/leads", nil)
	req.Header.Set("Origin", "http://127.0.0.1:8787")
	if !s.originAllowed(req) {
		t.Errorf("same origin should be allowed")
	}
}

func TestEmptyOriginAllowed(t *testing.T) {
	s := testServer("127.0.0.1", "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/leads", nil)
	if !s.originAllowed(req) {
		t.Errorf("missing origin should be allowed (non-browser client)")
	}
}
