// Package enrichment crawls a public business website for email, phone,
// booking links and social accounts.
package enrichment

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/ali-ulu/lead/leadscout-go/internal/audit"
	"github.com/ali-ulu/lead/leadscout-go/internal/netguard"
)

var emailRe = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
var phoneRe = regexp.MustCompile(`(?:\+?\d[\d\s().-]{7,}\d)`)

// SocialHosts mirrors the audit package host map.
var socialHosts = map[string][]string{
	"instagram": {"instagram.com"}, "facebook": {"facebook.com", "fb.com"},
	"linkedin": {"linkedin.com"}, "x": {"x.com", "twitter.com"},
	"youtube": {"youtube.com", "youtu.be"}, "tiktok": {"tiktok.com"},
	"telegram": {"t.me", "telegram.me"}, "whatsapp": {"wa.me", "api.whatsapp.com", "whatsapp.com"},
}

var followHints = []string{"contact", "iletisim", "iletişim", "kontakt", "about", "reservation", "booking", "appointment", "randevu", "termin"}

var fetchLimiter = netguard.NewLimiter(netguard.DefaultFetchInterval())

// Result is the enrichment outcome.
type Result struct {
	PagesScanned      []string          `json:"pages_scanned"`
	Emails            []string          `json:"emails"`
	Phones            []string          `json:"phones"`
	SocialLinks       map[string]string `json:"social_links"`
	BookingLinks      []string          `json:"booking_links"`
	Errors            []string          `json:"errors"`
	Complete          bool              `json:"complete"`
	EmailEnrichment   any               `json:"email_enrichment,omitempty"`
	EmailVerification any               `json:"email_verification,omitempty"`
}

type pageData struct {
	links        []string
	socials      map[string]string
	mailtos      []string
	tels         []string
	bookingLinks []string
}

func parsePage(base, doc string) *pageData {
	pd := &pageData{socials: map[string]string{}}
	root, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		return pd
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := ""
			for _, a := range n.Attr {
				if strings.EqualFold(a.Key, "href") {
					href = strings.TrimSpace(a.Val)
				}
			}
			if href != "" {
				if strings.HasPrefix(href, "mailto:") {
					pd.mailtos = append(pd.mailtos, strings.SplitN(href[7:], "?", 2)[0])
				} else if strings.HasPrefix(href, "tel:") {
					pd.tels = append(pd.tels, href[4:])
				} else {
					abs, err := url.Parse(base)
					if err == nil {
						ref, err2 := url.Parse(href)
						if err2 == nil {
							resolved := abs.ResolveReference(ref)
							if resolved.Scheme == "http" || resolved.Scheme == "https" {
								full := resolved.String()
								pd.links = append(pd.links, full)
								host := strings.ToLower(resolved.Hostname())
								for platform, hosts := range socialHosts {
									for _, h := range hosts {
										if host == h || strings.HasSuffix(host, "."+h) {
											if _, ok := pd.socials[platform]; !ok {
												pd.socials[platform] = full
											}
										}
									}
								}
								lower := strings.ToLower(full)
								for _, hint := range []string{"book", "reservation", "appointment", "randevu", "termin", "calendly"} {
									if strings.Contains(lower, hint) {
										pd.bookingLinks = append(pd.bookingLinks, full)
										break
									}
								}
							}
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return pd
}

func fetch(rawURL string, timeout time.Duration) (string, string, error) {
	normalized, err := audit.NormalizeURL(rawURL)
	if err != nil {
		return "", "", err
	}
	if err := netguard.AssertPublicURL(normalized); err != nil {
		return "", "", err
	}
	if netguard.RespectRobots() && !netguard.RobotsAllows(normalized, 5*time.Second) {
		return "", "", errRobots
	}
	fetchLimiter.Wait()
	client := netguard.NewPublicClient(timeout, false)
	req, _ := http.NewRequest(http.MethodGet, normalized, nil)
	req.Header.Set("User-Agent", netguard.UserAgent)
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	final := resp.Request.URL.String()
	if err := netguard.AssertPublicURL(final); err != nil {
		return "", "", err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1_200_000))
	return final, string(raw), nil
}

var errRobots = &robotsError{}

type robotsError struct{}

func (e *robotsError) Error() string { return "Blocked by robots.txt" }

// EnrichWebsite crawls up to maxPages same-host pages for contact data.
func EnrichWebsite(rawURL string, maxPages int) (Result, error) {
	start, err := audit.NormalizeURL(rawURL)
	if err != nil {
		return Result{}, err
	}
	rootHost := strings.TrimPrefix(hostOf(start), "www.")
	queue := []string{start}
	seen := map[string]bool{}
	emails := map[string]bool{}
	phones := map[string]bool{}
	socials := map[string]string{}
	booking := map[string]bool{}
	var pages []string
	var errs []string

	for len(queue) > 0 && len(pages) < maxPages {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		final, doc, err := fetch(current, 10*time.Second)
		if err != nil {
			errs = append(errs, current+": "+err.Error())
			continue
		}
		pages = append(pages, final)
		pd := parsePage(final, doc)
		for _, e := range pd.mailtos {
			emails[e] = true
		}
		for _, e := range emailRe.FindAllString(doc, -1) {
			emails[e] = true
		}
		for _, t := range pd.tels {
			phones[t] = true
		}
		for _, raw := range phoneRe.FindAllString(doc, -1) {
			digits := strings.Map(func(r rune) rune {
				if (r >= '0' && r <= '9') || r == '+' {
					return r
				}
				return -1
			}, raw)
			count := 0
			for _, r := range digits {
				if r >= '0' && r <= '9' {
					count++
				}
			}
			if count >= 8 {
				phones[digits] = true
			}
		}
		for k, v := range pd.socials {
			if _, ok := socials[k]; !ok {
				socials[k] = v
			}
		}
		for _, b := range pd.bookingLinks {
			booking[b] = true
		}
		for _, link := range pd.links {
			u, err := url.Parse(link)
			if err != nil {
				continue
			}
			if strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") != rootHost {
				continue
			}
			lower := strings.ToLower(link)
			for _, hint := range followHints {
				if strings.Contains(lower, hint) {
					if !seen[link] && !contains(queue, link) {
						queue = append(queue, link)
					}
					break
				}
			}
		}
	}

	return Result{
		PagesScanned: pages,
		Emails:       sortedLimited(emails, 20, true),
		Phones:       sortedLimited(phones, 20, false),
		SocialLinks:  socials,
		BookingLinks: firstNStrings(sortedSet(booking), 20),
		Errors:       firstNStrings(errs, 10),
		Complete:     len(pages) > 0,
	}, nil
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func sortedLimited(set map[string]bool, limit int, lower bool) []string {
	var out []string
	for k := range set {
		v := strings.TrimSpace(k)
		if lower {
			v = strings.ToLower(v)
		}
		out = append(out, v)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return firstNStrings(out, limit)
}

func sortedSet(set map[string]bool) []string {
	var out []string
	for k := range set {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func firstNStrings(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

var _ = os.Getenv
