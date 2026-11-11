package oauthmeta

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

func setup(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	os.Setenv("LEADSCOUT_DB", filepath.Join(dir, "oauth.db"))
	os.Setenv("LEADSCOUT_SCHEMA", filepath.Join("..", "..", "..", "sql", "schema.sql"))
	os.Setenv("LEADSCOUT_TOKEN_KEY", "test-token-key")
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
}

func makeLead(t *testing.T, sourceID string) int64 {
	t.Helper()
	ids, err := db.UpsertLeads([]*model.Lead{{
		Source: "test", SourceID: sourceID, Name: "Lead " + sourceID, Country: "TR",
		Category: "restaurant", WebsiteStatus: "missing", SocialLinks: map[string]string{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return ids[0]
}

func TestFacebookOAuthURLHasStateAndMessagingScopes(t *testing.T) {
	setup(t)
	os.Setenv("META_APP_ID", "123")
	os.Setenv("META_APP_SECRET", "secret")
	os.Setenv("META_FACEBOOK_REDIRECT_URI", "http://127.0.0.1:8787/api/v1/oauth/meta/facebook/callback")
	defer func() { os.Unsetenv("META_APP_ID"); os.Unsetenv("META_APP_SECRET") }()
	url, err := FacebookOAuthStartURL()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"client_id=123", "state=", "pages_messaging"} {
		if !strings.Contains(url, want) {
			t.Errorf("url missing %q: %s", want, url)
		}
	}
}

func TestInstagramOAuthURLUsesBusinessScopes(t *testing.T) {
	setup(t)
	os.Setenv("META_INSTAGRAM_APP_ID", "456")
	os.Setenv("META_INSTAGRAM_APP_SECRET", "secret")
	os.Setenv("META_INSTAGRAM_REDIRECT_URI", "http://127.0.0.1:8787/api/v1/oauth/meta/instagram/callback")
	defer func() {
		os.Unsetenv("META_INSTAGRAM_APP_ID")
		os.Unsetenv("META_INSTAGRAM_APP_SECRET")
	}()
	url, err := InstagramOAuthStartURL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "https://www.instagram.com/oauth/authorize?") {
		t.Errorf("unexpected prefix: %s", url)
	}
	for _, want := range []string{"instagram_business_basic", "instagram_business_manage_messages", "state="} {
		if !strings.Contains(url, want) {
			t.Errorf("url missing %q: %s", want, url)
		}
	}
}

func TestWebhookSignatureValidation(t *testing.T) {
	raw := []byte(`{"object":"page","entry":[]}`)
	secret := "top-secret"
	os.Setenv("META_APP_SECRET", secret)
	defer os.Unsetenv("META_APP_SECRET")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !VerifyWebhookSignature(raw, sig, "facebook") {
		t.Errorf("valid signature rejected")
	}
	if VerifyWebhookSignature(raw, "sha256=bad", "facebook") {
		t.Errorf("bad signature accepted")
	}
}

func TestSendMessageRecordsSentActivity(t *testing.T) {
	setup(t)
	leadID := makeLead(t, "send-1")
	if _, err := db.UpdateLead(leadID, map[string]any{"messaging_ids": map[string]string{"instagram": "IGSID-1"}}); err != nil {
		t.Fatal(err)
	}
	if err := saveConnection("instagram", "17841400000000000", "acct", "token",
		map[string]any{"api_base": "instagram"}, 0); err != nil {
		t.Fatal(err)
	}

	original := requestJSON
	requestJSON = func(method, endpoint string, data map[string]any, token string, form bool) (map[string]any, error) {
		if !strings.Contains(endpoint, "/messages") {
			t.Errorf("unexpected endpoint %s", endpoint)
		}
		return map[string]any{"recipient_id": "IGSID-1", "message_id": "mid.sent"}, nil
	}
	defer func() { requestJSON = original }()

	result, err := SendMessage(leadID, "instagram", nil, "Hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("ok = %v", result["ok"])
	}
	if result["external_id"] != "mid.sent" {
		t.Errorf("external_id = %v", result["external_id"])
	}
	lead, _ := db.GetLead(leadID)
	if lead.EngagementStatus != "sent" {
		t.Errorf("engagement = %q, want sent", lead.EngagementStatus)
	}
}

func TestMessagingEligibilityRequiresRealRecipientID(t *testing.T) {
	setup(t)
	leadID := makeLead(t, "elig-1")
	first, err := MessagingEligibility(leadID, "instagram")
	if err != nil {
		t.Fatal(err)
	}
	if eligible, _ := first["eligible"].(bool); eligible {
		t.Errorf("expected ineligible before linking")
	}
	if _, err := db.UpdateLead(leadID, map[string]any{"messaging_ids": map[string]string{"instagram": "IGSID-1"}}); err != nil {
		t.Fatal(err)
	}
	second, _ := MessagingEligibility(leadID, "instagram")
	if eligible, _ := second["eligible"].(bool); !eligible {
		t.Errorf("expected eligible after linking")
	}
	if rule, _ := second["rule"].(string); !strings.Contains(rule, "already messaged") {
		t.Errorf("rule = %q", rule)
	}
}

func TestWebhookPayloadLinksSenderAndRecordsReply(t *testing.T) {
	setup(t)
	leadID := makeLead(t, "hook-1")
	if _, err := db.UpdateLead(leadID, map[string]any{"messaging_ids": map[string]string{"instagram": "IGSID-9"}}); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"object": "instagram",
		"entry": []any{
			map[string]any{
				"messaging": []any{
					map[string]any{
						"sender":  map[string]any{"id": "IGSID-9"},
						"message": map[string]any{"text": "hi there", "mid": "mid.in"},
					},
				},
			},
		},
	}
	result := HandleWebhookPayload("instagram", payload)
	if result["handled"].(int) != 1 {
		t.Errorf("handled = %v", result["handled"])
	}
}

func TestWebhookLeavesUnknownSenderUnmatched(t *testing.T) {
	setup(t)
	payload := map[string]any{
		"entry": []any{
			map[string]any{
				"messaging": []any{
					map[string]any{
						"sender":  map[string]any{"id": "UNKNOWN"},
						"message": map[string]any{"text": "hi", "mid": "mid.x"},
					},
				},
			},
		},
	}
	result := HandleWebhookPayload("instagram", payload)
	if result["unmatched"].(int) != 1 {
		t.Errorf("unmatched = %v", result["unmatched"])
	}
}

func TestWebhookIgnoresEcho(t *testing.T) {
	setup(t)
	payload := map[string]any{
		"entry": []any{
			map[string]any{
				"messaging": []any{
					map[string]any{
						"sender":  map[string]any{"id": "X"},
						"message": map[string]any{"text": "echo", "is_echo": true},
					},
				},
			},
		},
	}
	result := HandleWebhookPayload("facebook", payload)
	if result["handled"].(int) != 0 || result["unmatched"].(int) != 0 {
		t.Errorf("echo should be skipped: %v", result)
	}
}

var _ = http.MethodGet
