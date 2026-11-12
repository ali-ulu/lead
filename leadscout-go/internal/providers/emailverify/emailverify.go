// Package emailverify scores an email address for outreach: syntax,
// disposable/role/free classification and an MX record check.
package emailverify

import (
	"context"
	"net"
	"regexp"
	"strings"
	"time"
)

var emailRe = regexp.MustCompile(`(?i)^[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}$`)

// AssetExtensions are filename types that a naive regex reads as a TLD, so
// "sprite@2x.png" or "icon@3x-480x358.jpg" look like addresses.
var AssetExtensions = map[string]bool{
	"png": true, "jpg": true, "jpeg": true, "gif": true, "svg": true,
	"webp": true, "ico": true, "bmp": true, "avif": true, "tiff": true,
	"css": true, "js": true, "mjs": true, "json": true, "xml": true,
	"txt": true, "pdf": true, "map": true, "webmanifest": true,
	"mp4": true, "webm": true, "mov": true, "avi": true, "mp3": true,
	"wav": true, "ogg": true,
	"woff": true, "woff2": true, "ttf": true, "otf": true, "eot": true,
	"zip": true, "gz": true, "tar": true, "rar": true, "7z": true,
}

// LooksLikeEmail reports whether a regex match is an address rather than a
// filename that merely contains an "@".
func LooksLikeEmail(address string) bool {
	local, domain, ok := strings.Cut(strings.ToLower(strings.TrimSpace(address)), "@")
	if !ok || local == "" {
		return false
	}
	dot := strings.LastIndex(domain, ".")
	if dot < 0 {
		return false
	}
	return !AssetExtensions[domain[dot+1:]]
}

// DisposableDomains are throwaway mailbox providers.
var DisposableDomains = map[string]bool{
	"mailinator.com": true, "guerrillamail.com": true, "10minutemail.com": true,
	"tempmail.com": true, "temp-mail.org": true, "throwawaymail.com": true,
	"yopmail.com": true, "trashmail.com": true, "sharklasers.com": true,
	"getnada.com": true, "maildrop.cc": true, "dispostable.com": true,
}

// FreeProviders are large consumer mailbox providers.
var FreeProviders = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "yahoo.com": true,
	"yandex.com": true, "yandex.ru": true, "hotmail.com": true,
	"outlook.com": true, "live.com": true, "icloud.com": true, "aol.com": true,
	"gmx.com": true, "gmx.de": true, "mail.com": true, "protonmail.com": true,
	"proton.me": true, "zoho.com": true,
}

// RoleLocalParts are shared inboxes rather than a named person.
var RoleLocalParts = map[string]bool{
	"info": true, "contact": true, "sales": true, "support": true, "admin": true,
	"office": true, "hello": true, "help": true, "billing": true,
	"postmaster": true, "webmaster": true, "noreply": true, "no-reply": true,
	"abuse": true, "marketing": true, "hr": true, "jobs": true, "careers": true,
	"enquiries": true, "inquiries": true,
}

// Result is the verification outcome for one address.
type Result struct {
	Email        string `json:"email"`
	ValidSyntax  bool   `json:"valid_syntax"`
	Domain       string `json:"domain,omitempty"`
	Disposable   bool   `json:"disposable"`
	RoleAccount  bool   `json:"role_account"`
	FreeProvider bool   `json:"free_provider"`
	MXFound      *bool  `json:"mx_found"`
	Score        int    `json:"score"`
	Status       string `json:"status"`
	Reason       string `json:"reason"`
}

// VerifyEmail scores a single address. Status is valid, risky or invalid.
// checkMX controls the DNS lookup so callers can stay offline in tests.
func VerifyEmail(email string, checkMX bool, timeout time.Duration) Result {
	address := strings.ToLower(strings.TrimSpace(email))
	if !emailRe.MatchString(address) || !LooksLikeEmail(address) {
		return Result{Email: address, Status: "invalid", Reason: "Malformed address"}
	}
	local, domain, _ := strings.Cut(address, "@")
	disposable := DisposableDomains[domain]
	role := RoleLocalParts[local]
	free := FreeProviders[domain]

	var mxFound *bool
	if checkMX {
		mxFound = lookupMX(domain, timeout)
	}

	score := 100
	var reasons []string
	if disposable {
		score -= 70
		reasons = append(reasons, "Disposable domain")
	}
	if role {
		score -= 15
		reasons = append(reasons, "Role account")
	}
	if free {
		score -= 5
		reasons = append(reasons, "Free provider")
	}
	if mxFound != nil && !*mxFound {
		score -= 60
		reasons = append(reasons, "No MX record")
	}
	if score < 0 {
		score = 0
	}

	status := "risky"
	switch {
	case (mxFound != nil && !*mxFound) || disposable:
		status = "invalid"
	case score >= 80:
		status = "valid"
	}
	reason := "Passed all checks"
	if len(reasons) > 0 {
		reason = strings.Join(reasons, "; ")
	}
	return Result{
		Email: address, ValidSyntax: true, Domain: domain,
		Disposable: disposable, RoleAccount: role, FreeProvider: free,
		MXFound: mxFound, Score: score, Status: status, Reason: reason,
	}
}

func lookupMX(domain string, timeout time.Duration) *bool {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	records, err := net.DefaultResolver.LookupMX(ctx, domain)
	found := err == nil && len(records) > 0
	return &found
}
