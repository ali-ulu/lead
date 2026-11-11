// Package oauthmeta handles Meta (Facebook/Instagram) OAuth, encrypted token
// storage, messaging eligibility and webhook processing for the local
// outreach workflow.
package oauthmeta

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/crm"
	"github.com/ali-ulu/lead/leadscout-go/internal/db"
)

// DefaultScopes mirrors the Python defaults.
const (
	FacebookDefaultScopes  = "pages_show_list,pages_messaging,pages_manage_metadata,pages_read_engagement"
	InstagramDefaultScopes = "instagram_business_basic,instagram_business_manage_messages"
)

// ErrNotConfigured signals missing Meta app credentials.
var ErrNotConfigured = errors.New("Meta OAuth is not configured")

// ErrInvalidState signals an unknown or mismatched OAuth state.
var ErrInvalidState = errors.New("Invalid OAuth state")

// requestJSON is the HTTP client used for Graph API calls; overridable in tests.
var requestJSON = defaultRequestJSON

func graphVersion() string { return strings.TrimSpace(os.Getenv("META_GRAPH_VERSION")) }

func facebookBase() string {
	if v := graphVersion(); v != "" {
		return "https://graph.facebook.com/" + v
	}
	return "https://graph.facebook.com"
}

func instagramBase() string {
	v := strings.TrimSpace(os.Getenv("META_INSTAGRAM_GRAPH_VERSION"))
	if v == "" {
		v = graphVersion()
	}
	if v != "" {
		return "https://graph.instagram.com/" + v
	}
	return "https://graph.instagram.com"
}

// MetaConfigured reports whether app credentials are present.
func MetaConfigured(provider string) bool {
	if strings.EqualFold(strings.TrimSpace(provider), "instagram") {
		appID := firstNonEmpty(os.Getenv("META_INSTAGRAM_APP_ID"), os.Getenv("META_APP_ID"))
		secret := firstNonEmpty(os.Getenv("META_INSTAGRAM_APP_SECRET"), os.Getenv("META_APP_SECRET"))
		return appID != "" && secret != ""
	}
	return os.Getenv("META_APP_ID") != "" && os.Getenv("META_APP_SECRET") != ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// --- token encryption -----------------------------------------------------

func tokenKey() ([]byte, error) {
	if configured := strings.TrimSpace(os.Getenv("LEADSCOUT_TOKEN_KEY")); configured != "" {
		if raw, err := base64.StdEncoding.DecodeString(configured); err == nil && len(raw) == 32 {
			return raw, nil
		}
		sum := sha256.Sum256([]byte(configured))
		return sum[:], nil
	}
	keyPath := filepath.Join(db.RootDir(), "data", "token.key")
	if raw, err := os.ReadFile(keyPath); err == nil {
		if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw))); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
		sum := sha256.Sum256(raw)
		return sum[:], nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	_ = os.MkdirAll(filepath.Dir(keyPath), 0o700)
	_ = os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(key)), 0o600)
	return key, nil
}

func encryptToken(token string) (string, error) {
	key, err := tokenKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(token), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptToken(encoded string) (string, error) {
	key, err := tokenKey()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("invalid ciphertext")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// --- OAuth state ----------------------------------------------------------

// CreateOAuthState stores a fresh state token for the provider.
func CreateOAuthState(provider string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "facebook" && provider != "instagram" {
		return "", errors.New("Unsupported OAuth provider.")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	state := base64.RawURLEncoding.EncodeToString(raw)
	conn, err := db.Conn()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if _, err := conn.Exec("INSERT INTO oauth_states(state,provider) VALUES (?,?)", state, provider); err != nil {
		return "", err
	}
	return state, nil
}

func consumeState(state, provider string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var storedProvider string
	err = conn.QueryRow("SELECT provider FROM oauth_states WHERE state=?", state).Scan(&storedProvider)
	if err != nil || storedProvider != provider {
		return ErrInvalidState
	}
	_, err = conn.Exec("DELETE FROM oauth_states WHERE state=?", state)
	return err
}

// FacebookOAuthStartURL builds the Facebook Login dialog URL.
func FacebookOAuthStartURL() (string, error) {
	appID := strings.TrimSpace(os.Getenv("META_APP_ID"))
	if appID == "" {
		return "", errors.New("META_APP_ID is not configured.")
	}
	redirect := firstNonEmpty(os.Getenv("META_FACEBOOK_REDIRECT_URI"), "http://127.0.0.1:8787/api/v1/oauth/meta/facebook/callback")
	scopes := firstNonEmpty(os.Getenv("META_FACEBOOK_SCOPES"), FacebookDefaultScopes)
	state, err := CreateOAuthState("facebook")
	if err != nil {
		return "", err
	}
	params := url.Values{
		"client_id": {appID}, "redirect_uri": {redirect}, "state": {state},
		"scope": {scopes}, "response_type": {"code"},
	}
	prefix := "https://www.facebook.com/dialog/oauth"
	if v := graphVersion(); v != "" {
		prefix = "https://www.facebook.com/" + v + "/dialog/oauth"
	}
	return prefix + "?" + params.Encode(), nil
}

// InstagramOAuthStartURL builds the Instagram Business Login URL.
func InstagramOAuthStartURL() (string, error) {
	appID := firstNonEmpty(os.Getenv("META_INSTAGRAM_APP_ID"), os.Getenv("META_APP_ID"))
	if appID == "" {
		return "", errors.New("META_INSTAGRAM_APP_ID or META_APP_ID is not configured.")
	}
	redirect := firstNonEmpty(os.Getenv("META_INSTAGRAM_REDIRECT_URI"), "http://127.0.0.1:8787/api/v1/oauth/meta/instagram/callback")
	scopes := firstNonEmpty(os.Getenv("META_INSTAGRAM_SCOPES"), InstagramDefaultScopes)
	state, err := CreateOAuthState("instagram")
	if err != nil {
		return "", err
	}
	params := url.Values{
		"client_id": {appID}, "redirect_uri": {redirect}, "state": {state},
		"scope": {scopes}, "response_type": {"code"},
	}
	return "https://www.instagram.com/oauth/authorize?" + params.Encode(), nil
}

// --- Graph HTTP -----------------------------------------------------------

func defaultRequestJSON(method, endpoint string, data map[string]any, token string, form bool) (map[string]any, error) {
	var body io.Reader
	headers := map[string]string{"Accept": "application/json", "User-Agent": "LeadScout/5.1"}
	if method == http.MethodPost {
		if form {
			values := url.Values{}
			for k, v := range data {
				values.Set(k, fmt.Sprint(v))
			}
			body = strings.NewReader(values.Encode())
			headers["Content-Type"] = "application/x-www-form-urlencoded"
		} else {
			raw, _ := json.Marshal(data)
			body = strings.NewReader(string(raw))
			headers["Content-Type"] = "application/json"
		}
	}
	if token != "" {
		sep := "?"
		if strings.Contains(endpoint, "?") {
			sep = "&"
		}
		endpoint = endpoint + sep + "access_token=" + url.QueryEscape(token)
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveConnection(provider, accountID, accountName, token string, metadata map[string]any, expiresIn int) error {
	enc, err := encryptToken(token)
	if err != nil {
		return err
	}
	var expiresAt *string
	if expiresIn > 0 {
		v := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)
		expiresAt = &v
	}
	metaJSON, _ := json.Marshal(metadata)
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Exec(`
		INSERT INTO oauth_connections(provider,account_id,account_name,access_token,token_expires_at,metadata)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(provider,account_id) DO UPDATE SET
		  account_name=excluded.account_name,
		  access_token=excluded.access_token,
		  token_expires_at=excluded.token_expires_at,
		  metadata=excluded.metadata,
		  updated_at=CURRENT_TIMESTAMP`,
		provider, accountID, accountName, enc, expiresAt, string(metaJSON))
	return err
}

// HandleFacebookCallback exchanges the code and stores connected Pages.
func HandleFacebookCallback(code, state string) ([]map[string]any, error) {
	if !MetaConfigured("facebook") {
		return nil, ErrNotConfigured
	}
	if err := consumeState(state, "facebook"); err != nil {
		return nil, err
	}
	appID := strings.TrimSpace(os.Getenv("META_APP_ID"))
	secret := strings.TrimSpace(os.Getenv("META_APP_SECRET"))
	redirect := firstNonEmpty(os.Getenv("META_FACEBOOK_REDIRECT_URI"), "http://127.0.0.1:8787/api/v1/oauth/meta/facebook/callback")
	params := url.Values{"client_id": {appID}, "client_secret": {secret}, "redirect_uri": {redirect}, "code": {code}}
	tokenData, err := requestJSON(http.MethodGet, facebookBase()+"/oauth/access_token?"+params.Encode(), nil, "", false)
	if err != nil {
		return nil, err
	}
	userToken, _ := tokenData["access_token"].(string)
	if userToken == "" {
		return nil, errors.New("Meta did not return a Facebook access token.")
	}
	pagesData, err := requestJSON(http.MethodGet,
		facebookBase()+"/me/accounts?fields=id,name,access_token,tasks,instagram_business_account{id,username}",
		nil, userToken, false)
	if err != nil {
		return nil, err
	}
	saved := []map[string]any{}
	pages, _ := pagesData["data"].([]any)
	for _, raw := range pages {
		page, _ := raw.(map[string]any)
		pageID := fmt.Sprint(page["id"])
		pageToken, _ := page["access_token"].(string)
		if pageToken == "" {
			pageToken = userToken
		}
		if pageID != "" && pageID != "<nil>" {
			_ = saveConnection("facebook", pageID, firstNonEmpty(fmt.Sprint(page["name"]), pageID), pageToken,
				map[string]any{"kind": "facebook_page", "tasks": page["tasks"]}, 0)
			saved = append(saved, map[string]any{"provider": "facebook", "account_id": pageID, "account_name": page["name"]})
		}
		if ig, ok := page["instagram_business_account"].(map[string]any); ok {
			igID := fmt.Sprint(ig["id"])
			if igID != "" && igID != "<nil>" {
				_ = saveConnection("instagram", igID, firstNonEmpty(fmt.Sprint(ig["username"]), igID), pageToken,
					map[string]any{"kind": "instagram_via_facebook", "page_id": pageID, "username": ig["username"], "api_base": "facebook"}, 0)
				saved = append(saved, map[string]any{"provider": "instagram", "account_id": igID, "account_name": ig["username"]})
			}
		}
	}
	return saved, nil
}

// HandleInstagramCallback exchanges the code for a long-lived token.
func HandleInstagramCallback(code, state string) ([]map[string]any, error) {
	if !MetaConfigured("instagram") {
		return nil, ErrNotConfigured
	}
	if err := consumeState(state, "instagram"); err != nil {
		return nil, err
	}
	appID := firstNonEmpty(os.Getenv("META_INSTAGRAM_APP_ID"), os.Getenv("META_APP_ID"))
	secret := firstNonEmpty(os.Getenv("META_INSTAGRAM_APP_SECRET"), os.Getenv("META_APP_SECRET"))
	redirect := firstNonEmpty(os.Getenv("META_INSTAGRAM_REDIRECT_URI"), "http://127.0.0.1:8787/api/v1/oauth/meta/instagram/callback")

	short, err := requestJSON(http.MethodPost, "https://api.instagram.com/oauth/access_token", map[string]any{
		"client_id": appID, "client_secret": secret, "grant_type": "authorization_code",
		"redirect_uri": redirect, "code": code,
	}, "", true)
	if err != nil {
		return nil, err
	}
	shortToken, _ := short["access_token"].(string)
	if shortToken == "" {
		return nil, errors.New("Instagram did not return an access token.")
	}
	longParams := url.Values{"grant_type": {"ig_exchange_token"}, "client_secret": {secret}, "access_token": {shortToken}}
	longData, err := requestJSON(http.MethodGet, "https://graph.instagram.com/access_token?"+longParams.Encode(), nil, "", false)
	if err != nil {
		longData = map[string]any{}
	}
	token := shortToken
	if v, ok := longData["access_token"].(string); ok && v != "" {
		token = v
	}
	expiresIn := int(asFloat(longData["expires_in"]))
	if expiresIn == 0 {
		expiresIn = int(asFloat(short["expires_in"]))
	}
	profile, err := requestJSON(http.MethodGet, instagramBase()+"/me?fields=id,username", nil, token, false)
	if err != nil {
		return nil, err
	}
	accountID := firstNonEmpty(fmt.Sprint(profile["id"]), fmt.Sprint(short["user_id"]))
	if accountID == "" || accountID == "<nil>" {
		return nil, errors.New("Instagram account id was not returned.")
	}
	username := firstNonEmpty(fmt.Sprint(profile["username"]), accountID)
	_ = saveConnection("instagram", accountID, username, token,
		map[string]any{"kind": "instagram_login", "username": profile["username"], "api_base": "instagram",
			"scopes": firstNonEmpty(os.Getenv("META_INSTAGRAM_SCOPES"), InstagramDefaultScopes)}, expiresIn)
	return []map[string]any{{"provider": "instagram", "account_id": accountID, "account_name": username}}, nil
}

// ListConnections returns stored connections without exposing tokens.
func ListConnections() ([]map[string]any, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.Query("SELECT id,provider,account_id,account_name,token_expires_at,metadata,created_at,updated_at FROM oauth_connections ORDER BY provider,account_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var provider, accountID, accessToken string
		var accountName, expiresAt, metadata, createdAt, updatedAt *string
		if err := rows.Scan(&id, &provider, &accountID, &accountName, &accessToken, &expiresAt, &metadata, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		meta := map[string]any{}
		if metadata != nil {
			_ = json.Unmarshal([]byte(*metadata), &meta)
		}
		out = append(out, map[string]any{
			"id": id, "provider": provider, "account_id": accountID, "account_name": accountName,
			"token_expires_at": expiresAt, "metadata": meta, "created_at": createdAt, "updated_at": updatedAt,
		})
	}
	return out, rows.Err()
}

func connection(provider string, connectionID *int64) (map[string]any, string, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, "", err
	}
	defer conn.Close()
	query := "SELECT id,provider,account_id,account_name,access_token,metadata FROM oauth_connections WHERE provider=? ORDER BY updated_at DESC LIMIT 1"
	args := []any{provider}
	if connectionID != nil {
		query = "SELECT id,provider,account_id,account_name,access_token,metadata FROM oauth_connections WHERE id=? AND provider=?"
		args = []any{*connectionID, provider}
	}
	var id int64
	var prov, accountID, accessToken string
	var accountName, metadata *string
	if err := conn.QueryRow(query, args...).Scan(&id, &prov, &accountID, &accountName, &accessToken, &metadata); err != nil {
		return nil, "", fmt.Errorf("No connected %s account.", provider)
	}
	meta := map[string]any{}
	if metadata != nil {
		_ = json.Unmarshal([]byte(*metadata), &meta)
	}
	token, err := decryptToken(accessToken)
	if err != nil {
		return nil, "", err
	}
	return map[string]any{"id": id, "provider": prov, "account_id": accountID, "account_name": accountName, "metadata": meta}, token, nil
}

// Disconnect removes a stored connection.
func Disconnect(connectionID int64) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Exec("DELETE FROM oauth_connections WHERE id=?", connectionID)
	return err
}

// --- messaging ------------------------------------------------------------

// MessagingEligibility reports whether a lead has a sendable recipient id.
func MessagingEligibility(leadID int64, provider string) (map[string]any, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	lead, err := db.GetLead(leadID)
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	recipient := lead.MessagingIDs[provider]
	var rule string
	switch provider {
	case "instagram":
		rule = "Instagram API can reply only when the Instagram user has already messaged the connected professional account."
	case "facebook":
		rule = "Messenger requires an eligible Page-scoped recipient and messaging-window/permission eligibility."
	default:
		return nil, errors.New("provider must be facebook or instagram")
	}
	var recipientOut any
	if recipient != "" {
		recipientOut = recipient
	}
	return map[string]any{
		"provider":       provider,
		"eligible":       recipient != "" && !lead.DoNotContact,
		"recipient_id":   recipientOut,
		"do_not_contact": lead.DoNotContact,
		"rule":           rule,
	}, nil
}

// LinkRecipient stores a recipient id for a lead and records an activity.
func LinkRecipient(leadID int64, provider, recipientID string) (map[string]any, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "facebook" && provider != "instagram" {
		return nil, errors.New("provider must be facebook or instagram")
	}
	lead, err := db.GetLead(leadID)
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	recipientID = strings.TrimSpace(recipientID)
	if recipientID == "" {
		return nil, errors.New("recipient_id is required")
	}
	ids := lead.MessagingIDs
	if ids == nil {
		ids = map[string]string{}
	}
	ids[provider] = recipientID
	updated, err := db.UpdateLead(leadID, map[string]any{"messaging_ids": ids})
	if err != nil {
		return nil, err
	}
	_, _ = crm.AddActivity(leadID, "oauth", provider, "recipient_linked", "", "", "", map[string]any{"recipient_id": recipientID})
	eligibility, _ := MessagingEligibility(leadID, provider)
	return map[string]any{"lead": updated, "eligibility": eligibility}, nil
}

func sendEndpoint(conn map[string]any, provider string) string {
	senderID := url.QueryEscape(fmt.Sprint(conn["account_id"]))
	meta, _ := conn["metadata"].(map[string]any)
	if provider == "instagram" && meta["api_base"] == "instagram" {
		return instagramBase() + "/" + senderID + "/messages"
	}
	return facebookBase() + "/" + senderID + "/messages"
}

// SendMessage posts a message and records the resulting activity.
func SendMessage(leadID int64, provider string, recipientID *string, text string, connectionID *int64) (map[string]any, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "facebook" && provider != "instagram" {
		return nil, errors.New("provider must be facebook or instagram")
	}
	lead, err := db.GetLead(leadID)
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	if lead.DoNotContact {
		return nil, errors.New("Lead is marked do-not-contact.")
	}
	recipient := ""
	if recipientID != nil {
		recipient = strings.TrimSpace(*recipientID)
	}
	if recipient == "" {
		recipient = lead.MessagingIDs[provider]
	}
	if recipient == "" {
		eligibility, _ := MessagingEligibility(leadID, provider)
		rule, _ := eligibility["rule"].(string)
		return nil, fmt.Errorf("No eligible %s recipient/conversation id is stored for this lead. %s A public username/profile URL is not a sendable recipient id.", provider, rule)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("Message cannot be empty.")
	}
	conn, token, err := connection(provider, connectionID)
	if err != nil {
		return nil, err
	}
	result, err := requestJSON(http.MethodPost, sendEndpoint(conn, provider), map[string]any{
		"recipient": map[string]any{"id": recipient},
		"message":   map[string]any{"text": text},
	}, token, false)
	if err != nil {
		return nil, err
	}
	externalID := firstNonEmpty(fmt.Sprint(result["message_id"]), fmt.Sprint(result["id"]))
	if externalID == "<nil>" {
		externalID = ""
	}
	eligibility, _ := MessagingEligibility(leadID, provider)
	_, _ = crm.AddActivity(leadID, "message", provider, "sent", "out", text, externalID, map[string]any{
		"connection_id": conn["id"], "recipient_id": recipient, "response": result,
		"platform_rule": eligibility["rule"],
	})
	ids := lead.MessagingIDs
	if ids == nil {
		ids = map[string]string{}
	}
	ids[provider] = recipient
	_, _ = db.UpdateLead(leadID, map[string]any{"messaging_ids": ids, "engagement_status": "sent"})
	return map[string]any{"ok": true, "provider": provider, "recipient_id": recipient, "external_id": externalID, "response": result}, nil
}

// --- webhooks -------------------------------------------------------------

// VerifyWebhookSignature validates the X-Hub-Signature-256 header.
func VerifyWebhookSignature(rawBody []byte, signature, provider string) bool {
	secret := ""
	if strings.EqualFold(strings.TrimSpace(provider), "instagram") {
		secret = firstNonEmpty(os.Getenv("META_INSTAGRAM_APP_SECRET"), os.Getenv("META_APP_SECRET"))
	} else {
		secret = strings.TrimSpace(os.Getenv("META_APP_SECRET"))
	}
	if secret == "" || signature == "" || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ReceiveWebhookEvent records an inbound reply and links the sender.
func ReceiveWebhookEvent(provider string, leadID int64, senderID, text, externalID string) (crm.Activity, error) {
	lead, err := db.GetLead(leadID)
	if err != nil {
		return crm.Activity{}, errors.New("Lead not found.")
	}
	ids := lead.MessagingIDs
	if ids == nil {
		ids = map[string]string{}
	}
	ids[provider] = senderID
	_, _ = db.UpdateLead(leadID, map[string]any{"messaging_ids": ids})
	return crm.AddActivity(leadID, "reply", provider, "received", "in", text, externalID, nil)
}

func leadForSender(provider, senderID string) (*int64, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var id int64
	err = conn.QueryRow("SELECT id FROM leads WHERE json_extract(messaging_ids, ?) = ? LIMIT 1",
		"$."+provider, senderID).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// HandleWebhookPayload processes a Meta webhook payload.
func HandleWebhookPayload(provider string, payload map[string]any) map[string]any {
	if provider != "facebook" && provider != "instagram" {
		provider = "facebook"
	}
	handled, unmatched, statusUpdates := 0, 0, 0

	entries, _ := payload["entry"].([]any)
	for _, rawEntry := range entries {
		entry, _ := rawEntry.(map[string]any)
		var events []any
		if messaging, ok := entry["messaging"].([]any); ok {
			events = append(events, messaging...)
		}
		if changes, ok := entry["changes"].([]any); ok {
			for _, rawChange := range changes {
				change, _ := rawChange.(map[string]any)
				if value, ok := change["value"].(map[string]any); ok {
					if messages, ok := value["messages"].([]any); ok {
						events = append(events, messages...)
					}
				}
			}
		}

		for _, rawEvent := range events {
			event, _ := rawEvent.(map[string]any)
			if delivery, ok := event["delivery"].(map[string]any); ok {
				if mids, ok := delivery["mids"].([]any); ok {
					for _, rawMid := range mids {
						if updated, _ := crm.UpdateDeliveryStatus(fmt.Sprint(rawMid), "delivered"); updated {
							statusUpdates++
						}
					}
				}
			}
			if _, hasRead := event["read"]; hasRead {
				sender := senderOf(event)
				if sender != "" {
					if leadID, err := leadForSender(provider, sender); err == nil {
						_, _ = crm.AddActivity(*leadID, "status", provider, "read", "", "", "", nil)
						statusUpdates++
					}
				}
			}
			sender := senderOf(event)
			var text, mid string
			isEcho := false
			switch message := event["message"].(type) {
			case string:
				text = message
				mid = fmt.Sprint(event["id"])
			case map[string]any:
				text = fmt.Sprint(message["text"])
				mid = firstNonEmpty(fmt.Sprint(message["mid"]), fmt.Sprint(event["id"]))
				isEcho, _ = message["is_echo"].(bool)
			}
			if text == "<nil>" {
				text = ""
			}
			if mid == "<nil>" {
				mid = ""
			}
			if isEcho || sender == "" || text == "" {
				continue
			}
			leadID, err := leadForSender(provider, sender)
			if err != nil {
				unmatched++
				continue
			}
			if _, err := ReceiveWebhookEvent(provider, *leadID, sender, text, mid); err == nil {
				handled++
			}
		}
	}
	return map[string]any{"ok": true, "handled": handled, "unmatched": unmatched, "status_updates": statusUpdates}
}

func senderOf(event map[string]any) string {
	if sender, ok := event["sender"].(map[string]any); ok {
		if id := fmt.Sprint(sender["id"]); id != "" && id != "<nil>" {
			return id
		}
	}
	if from := fmt.Sprint(event["from"]); from != "" && from != "<nil>" {
		return from
	}
	return ""
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}
