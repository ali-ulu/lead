// Package audit fetches a business website, extracts markup/performance
// signals and turns them into visibility evidence and a website status.
package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/ali-ulu/lead/leadscout-go/internal/cache"
	"github.com/ali-ulu/lead/leadscout-go/internal/config"
	"github.com/ali-ulu/lead/leadscout-go/internal/netguard"
	"github.com/ali-ulu/lead/leadscout-go/internal/visibility"
)

// SocialHosts maps a platform to the hosts that identify it.
var SocialHosts = map[string][]string{
	"instagram": {"instagram.com"},
	"facebook":  {"facebook.com", "fb.com"},
	"linkedin":  {"linkedin.com"},
	"x":         {"x.com", "twitter.com"},
	"youtube":   {"youtube.com", "youtu.be"},
	"tiktok":    {"tiktok.com"},
	"telegram":  {"t.me", "telegram.me"},
	"whatsapp":  {"wa.me", "api.whatsapp.com", "whatsapp.com"},
}

var auditLimiter = netguard.NewLimiter(netguard.DefaultAuditInterval())

// TrackingMarkers maps an ad/analytics vendor to the strings that identify it.
// A business that runs these has some marketing maturity, which is a useful
// qualifier when prospecting. Matched against script sources and inline bodies.
var TrackingMarkers = map[string][]string{
	"google_analytics": {"googletagmanager.com", "google-analytics.com", "gtag(", "ga('", `ga("`},
	"google_ads":       {"googleadservices", "google_conversion", "googleads", "doubleclick.net"},
	"meta_pixel":       {"connect.facebook.net", "fbq(", "fbevents.js"},
	"hotjar":           {"hotjar", "hj("},
	"matomo":           {"matomo.", "piwik.", "_paq"},
	"linkedin_insight": {"snap.licdn.com", "linkedin insight"},
	"tiktok_pixel":     {"analytics.tiktok.com", "ttq."},
	"clarity":          {"clarity.ms"},
	"yandex_metrica":   {"mc.yandex", "ym("},
}

// DetectTracking returns the ad/analytics vendors detected on the page.
func DetectTracking(scriptSrcs, inlineScripts []string) []string {
	haystack := strings.ToLower(strings.Join(append(append([]string{}, scriptSrcs...), inlineScripts...), " "))
	if strings.TrimSpace(haystack) == "" {
		return nil
	}
	var found []string
	for vendor, markers := range TrackingMarkers {
		for _, marker := range markers {
			if strings.Contains(haystack, marker) {
				found = append(found, vendor)
				break
			}
		}
	}
	sort.Strings(found)
	return found
}

// NormalizeURL validates and returns an absolute http(s) URL.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("Website URL is empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("Only http/https URLs are supported")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("Only http/https URLs are supported")
	}
	return raw, nil
}

// SpeedProxy maps time-to-first-byte (ms) to a smooth 0-100 performance proxy.
func SpeedProxy(ms int) int {
	if ms <= 0 {
		return 90
	}
	v := 100 / (1 + math.Pow(float64(ms)/2500.0, 1.15))
	return int(math.Max(5, math.Min(100, math.Round(v))))
}

// signals holds the extracted markup evidence.
type signals struct {
	viewport      bool
	tel           bool
	booking       bool
	contact       bool
	forms         int
	h1            int
	metaDesc      bool
	canonical     string
	robotsNoindex bool
	metaAuthor    bool
	h2            int
	h3            int
	semanticMain  bool
	hrefs         []string
	headingTexts  []string
	textParts     []string
	jsonLD        []string
	timeValues    []string
	scriptSrcs    []string
	inlineScripts []string
	images        int
	altImages     int
	socialLinks   map[string]string
}

var wsRe = regexp.MustCompile(`\s+`)

func cleanText(s string) string { return strings.TrimSpace(wsRe.ReplaceAllString(s, " ")) }

func parseSignals(doc string) *signals {
	s := &signals{socialLinks: map[string]string{}}
	root, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		return s
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "meta":
				name := strings.ToLower(attr(n, "name"))
				content := attr(n, "content")
				if name == "viewport" {
					s.viewport = true
				}
				if name == "description" && strings.TrimSpace(content) != "" {
					s.metaDesc = true
				}
				if (name == "robots" || name == "googlebot") && strings.Contains(strings.ToLower(content), "noindex") {
					s.robotsNoindex = true
				}
				if name == "author" && strings.TrimSpace(content) != "" {
					s.metaAuthor = true
				}
			case "a":
				href := strings.TrimSpace(attr(n, "href"))
				if href != "" {
					s.hrefs = append(s.hrefs, href)
				}
				if strings.Contains(strings.ToLower(attr(n, "rel")), "author") {
					s.metaAuthor = true
				}
				lower := strings.ToLower(href)
				textish := strings.ToLower(href + " " + attr(n, "title"))
				if strings.HasPrefix(lower, "tel:") {
					s.tel = true
				}
				for _, x := range []string{"contact", "iletisim", "iletişim", "kontakt", "contato", "رابطہ"} {
					if strings.Contains(lower, x) {
						s.contact = true
						break
					}
				}
				for _, x := range []string{"book", "appointment", "reservation", "randevu", "termin", "reserve"} {
					if strings.Contains(lower+" "+textish, x) {
						s.booking = true
						break
					}
				}
				if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
					if u, err := url.Parse(href); err == nil {
						host := strings.ToLower(u.Hostname())
						for platform, hosts := range SocialHosts {
							for _, item := range hosts {
								if host == item || strings.HasSuffix(host, "."+item) {
									if _, ok := s.socialLinks[platform]; !ok {
										s.socialLinks[platform] = href
									}
								}
							}
						}
					}
				}
			case "form":
				s.forms++
			case "h1":
				s.h1++
			case "h2":
				s.h2++
			case "h3":
				s.h3++
			case "main", "article":
				s.semanticMain = true
			case "time":
				if dt := attr(n, "datetime"); dt != "" {
					s.timeValues = append(s.timeValues, dt)
				}
			case "link":
				rel := strings.ToLower(attr(n, "rel"))
				if strings.Contains(rel, "canonical") && attr(n, "href") != "" {
					s.canonical = strings.TrimSpace(attr(n, "href"))
				}
			case "script":
				src := strings.TrimSpace(attr(n, "src"))
				if src != "" {
					s.scriptSrcs = append(s.scriptSrcs, src)
				}
				if strings.Contains(strings.ToLower(attr(n, "type")), "application/ld+json") {
					var buf strings.Builder
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.TextNode {
							buf.WriteString(c.Data)
						}
					}
					if block := strings.TrimSpace(buf.String()); block != "" {
						s.jsonLD = append(s.jsonLD, block)
					}
				} else {
					// Inline script bodies carry the pixel/analytics calls
					// (gtag(...), fbq(...)) that never appear in a src URL.
					var buf strings.Builder
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.TextNode {
							buf.WriteString(c.Data)
						}
					}
					if body := strings.TrimSpace(buf.String()); body != "" {
						s.inlineScripts = append(s.inlineScripts, body)
					}
				}
			case "img":
				s.images++
				if strings.TrimSpace(attr(n, "alt")) != "" {
					s.altImages++
				}
			}
			if n.Data == "h1" || n.Data == "h2" || n.Data == "h3" || n.Data == "h4" {
				var buf strings.Builder
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.TextNode {
						buf.WriteString(c.Data)
					}
				}
				if text := cleanText(buf.String()); text != "" {
					s.headingTexts = append(s.headingTexts, text)
				}
			}
		}
		if n.Type == html.TextNode {
			if text := cleanText(n.Data); text != "" {
				s.textParts = append(s.textParts, text)
			}
		}
		// Script bodies are collected above (as JSON-LD or inline JS) and
		// must not be counted as page text.
		if n.Type == html.ElementNode && n.Data == "script" {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return s
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

func walkJSONLD(value any, types, fields map[string]bool) {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			walkJSONLD(item, types, fields)
		}
	case map[string]any:
		if rawType, ok := v["@type"]; ok {
			switch t := rawType.(type) {
			case string:
				types[t] = true
			case []any:
				for _, x := range t {
					if s, ok := x.(string); ok {
						types[s] = true
					}
				}
			}
		}
		for k, val := range v {
			switch val.(type) {
			case nil:
			case string:
				if val != "" {
					fields[k] = true
				}
			case []any:
				if len(val.([]any)) > 0 {
					fields[k] = true
				}
			case map[string]any:
				if len(val.(map[string]any)) > 0 {
					fields[k] = true
				}
			}
			walkJSONLD(val, types, fields)
		}
	}
}

func structuredData(blocks []string) ([]string, []string) {
	types := map[string]bool{}
	fields := map[string]bool{}
	for _, block := range blocks {
		var parsed any
		if err := json.Unmarshal([]byte(block), &parsed); err != nil {
			continue
		}
		walkJSONLD(parsed, types, fields)
	}
	return sortedKeys(types), sortedKeys(fields)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
var copyrightRe = regexp.MustCompile(`(?i)(?:©|copyright[^0-9]{0,15})(20\d{2})`)
var wordRe = regexp.MustCompile(`[\pL\pN'-]+`)
var factRe = regexp.MustCompile(`(?:\d+[\d.,%]*|[$€£₺₨]\s?\d+)`)

var questionPrefixes = []string{
	"what ", "why ", "how ", "when ", "where ", "who ", "can ", "do ", "does ", "is ", "are ",
	"nedir ", "nasıl ", "neden ", "ne zaman ", "nerede ", "kim ", "کیا ", "کیسے ", "ڇا ", "ڪيئن ",
}

// Result is the audit outcome stored on the lead.
type Result struct {
	Reachable           bool                `json:"reachable"`
	Status              int                 `json:"status,omitempty"`
	FinalURL            string              `json:"final_url,omitempty"`
	HasHTTPS            bool                `json:"has_https,omitempty"`
	ResponseMS          int                 `json:"response_ms,omitempty"`
	TTFBMS              int                 `json:"ttfb_ms,omitempty"`
	PerformanceScore    *int                `json:"performance_score,omitempty"`
	AccessibilityScore  *int                `json:"accessibility_score,omitempty"`
	BestPracticesScore  *int                `json:"best_practices_score,omitempty"`
	SEOScore            *int                `json:"seo_score,omitempty"`
	LighthouseSEOScore  *int                `json:"lighthouse_seo_score,omitempty"`
	AEOScore            *int                `json:"aeo_score,omitempty"`
	GEOScore            *int                `json:"geo_score,omitempty"`
	AIVisibilityScore   *int                `json:"ai_visibility_score,omitempty"`
	OpportunityGapScore *int                `json:"opportunity_gap_score,omitempty"`
	ScoreConfidence     map[string]string   `json:"score_confidence,omitempty"`
	VisibilityReasons   map[string][]string `json:"visibility_reasons,omitempty"`
	VisibilitySignals   map[string]any      `json:"visibility_signals,omitempty"`
	AuditEngine         string              `json:"audit_engine,omitempty"`
	FieldData           map[string]any      `json:"field_data,omitempty"`
	Indexable           bool                `json:"indexable"`
	MobileOK            bool                `json:"mobile_ok"`
	HasCTA              bool                `json:"has_cta"`
	HasBooking          bool                `json:"has_booking"`
	Title               string              `json:"title,omitempty"`
	LatestCopyrightYear *int                `json:"latest_copyright_year,omitempty"`
	QualityFlags        []string            `json:"quality_flags,omitempty"`
	TrackingVendors     []string            `json:"tracking_vendors,omitempty"`
	SocialLinks         map[string]string   `json:"social_links"`
	WebsiteStatus       string              `json:"website_status"`
	Error               string              `json:"error,omitempty"`
	RobotsBlocked       bool                `json:"robots_blocked,omitempty"`
	Note                string              `json:"note,omitempty"`
	Cached              bool                `json:"cached,omitempty"`
}

// AuditURL fetches and analyses a website.
func AuditURL(rawURL string, timeout time.Duration) (Result, error) {
	normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return Result{}, err
	}
	if cache.Enabled() {
		var cached Result
		if cache.Get("audit", []any{normalized}, &cached) {
			cached.Cached = true
			return cached, nil
		}
	}

	if err := netguard.AssertPublicURL(normalized); err != nil {
		if netguard.IsUnresolved(err) {
			// A retired domain is a finding, not an error.
			return Result{Reachable: false, WebsiteStatus: "dead", Error: err.Error(), SocialLinks: map[string]string{}}, nil
		}
		return Result{}, err
	}
	if netguard.RespectRobots() && !netguard.RobotsAllows(normalized, 5*time.Second) {
		return Result{
			Reachable: false, WebsiteStatus: "blocked", Error: "Blocked by robots.txt",
			Indexable: false, RobotsBlocked: true, SocialLinks: map[string]string{},
		}, nil
	}

	auditLimiter.Wait()
	client := netguard.NewPublicClient(timeout, false)
	req, _ := http.NewRequest(http.MethodGet, normalized, nil)
	req.Header.Set("User-Agent", netguard.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return Result{Reachable: false, Error: err.Error(), WebsiteStatus: "weak", SocialLinks: map[string]string{}}, nil
	}
	defer resp.Body.Close()
	ttfb := int(time.Since(start).Milliseconds())
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1_800_000))
	elapsed := int(time.Since(start).Milliseconds())
	body := string(raw)
	final := resp.Request.URL.String()
	headerNoindex := strings.Contains(strings.ToLower(resp.Header.Get("X-Robots-Tag")), "noindex")

	if err := netguard.AssertPublicURL(final); err != nil {
		return Result{Reachable: false, Error: err.Error(), WebsiteStatus: "weak", SocialLinks: map[string]string{}}, nil
	}

	lighthouse := runLighthouse(final)
	// No local Chrome/Lighthouse: try the PageSpeed Insights API, which runs the
	// same audit remotely and adds CrUX field data.
	var fieldData map[string]any
	usedPageSpeed := false
	if lighthouse == nil {
		lighthouse, fieldData = runPageSpeed(final)
		usedPageSpeed = lighthouse != nil
	}
	effective := body
	if lighthouse != nil && lighthouse.RenderedHTML != "" && len(lighthouse.RenderedHTML) > len(body) {
		effective = lighthouse.RenderedHTML
	}

	s := parseSignals(effective)
	title := ""
	if m := titleRe.FindStringSubmatch(effective); m != nil {
		title = cleanText(m[1])
	}
	var latestYear *int
	for _, m := range copyrightRe.FindAllStringSubmatch(effective, -1) {
		var y int
		fmt.Sscanf(m[1], "%d", &y)
		if latestYear == nil || y > *latestYear {
			v := y
			latestYear = &v
		}
	}

	var qualityFlags []string
	if !s.viewport {
		qualityFlags = append(qualityFlags, "No mobile viewport")
	}
	if !(s.tel || s.contact || s.forms > 0) {
		qualityFlags = append(qualityFlags, "No clear contact CTA")
	}
	if !s.booking {
		qualityFlags = append(qualityFlags, "No booking/reservation signal")
	}
	if !strings.HasPrefix(final, "https://") {
		qualityFlags = append(qualityFlags, "HTTPS missing")
	}
	if ttfb > config.SpeedFlagMS {
		qualityFlags = append(qualityFlags, "Slow first response")
	}
	if title == "" {
		qualityFlags = append(qualityFlags, "Missing page title")
	}
	if !s.metaDesc {
		qualityFlags = append(qualityFlags, "Missing meta description")
	}
	if s.images > 0 && float64(s.altImages)/float64(max(1, s.images)) < 0.5 {
		qualityFlags = append(qualityFlags, "Many images lack alt text")
	}
	if latestYear != nil && *latestYear <= 2022 {
		qualityFlags = append(qualityFlags, fmt.Sprintf("Old copyright signal (%d)", *latestYear))
	}

	var performance *int
	var accessibility, bestPractices, lighthouseSEO *int
	if lighthouse != nil {
		performance = lighthouse.Performance
		accessibility = lighthouse.Accessibility
		bestPractices = lighthouse.BestPractices
		lighthouseSEO = lighthouse.SEO
	}
	if performance == nil {
		p := SpeedProxy(ttfb)
		performance = &p
	}

	schemaTypes, schemaFields := structuredData(s.jsonLD)
	businessSchema := false
	for _, t := range schemaTypes {
		if t == "LocalBusiness" || strings.HasSuffix(t, "Business") ||
			t == "Restaurant" || t == "Dentist" || t == "MedicalBusiness" ||
			t == "ProfessionalService" || t == "Store" || t == "Hotel" {
			businessSchema = true
		}
	}
	organizationSchema := false
	for _, t := range schemaTypes {
		if t == "Organization" {
			organizationSchema = true
		}
	}

	bodyText := strings.Join(s.textParts, " ")
	words := wordRe.FindAllString(bodyText, -1)
	questionHeadings := 0
	for _, h := range s.headingTexts {
		if strings.Contains(h, "?") {
			questionHeadings++
			continue
		}
		lower := strings.ToLower(h)
		for _, p := range questionPrefixes {
			if strings.HasPrefix(lower, p) {
				questionHeadings++
				break
			}
		}
	}

	finalHost := strings.TrimPrefix(hostOnly(final), "www.")
	socialHostSet := map[string]bool{}
	for _, hosts := range SocialHosts {
		for _, h := range hosts {
			socialHostSet[h] = true
		}
	}
	externalHosts := map[string]bool{}
	for _, href := range s.hrefs {
		if !(strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://")) {
			continue
		}
		host := strings.TrimPrefix(hostOnly(href), "www.")
		if host == "" || host == finalHost || strings.HasSuffix(host, "."+finalHost) {
			continue
		}
		skip := false
		for social := range socialHostSet {
			if host == social || strings.HasSuffix(host, "."+social) {
				skip = true
				break
			}
		}
		if !skip {
			externalHosts[host] = true
		}
	}
	factSignals := len(factRe.FindAllString(bodyText, -1))
	yearNow := time.Now().UTC().Year()
	freshSignal := false
	for _, v := range s.timeValues {
		if strings.Contains(v, fmt.Sprintf("%d", yearNow)) || strings.Contains(v, fmt.Sprintf("%d", yearNow-1)) {
			freshSignal = true
			break
		}
	}
	if latestYear != nil && *latestYear >= yearNow-1 {
		freshSignal = true
	}
	if latestYear != nil && *latestYear < yearNow-2 {
		freshSignal = false
	}

	trackingVendors := DetectTracking(s.scriptSrcs, s.inlineScripts)

	evidence := visibility.Evidence{
		Indexable:          !s.robotsNoindex && !headerNoindex,
		RobotsNoindex:      s.robotsNoindex,
		HeaderNoindex:      headerNoindex,
		Title:              title,
		MetaDescription:    s.metaDesc,
		H1Count:            s.h1,
		H2H3Count:          s.h2 + s.h3,
		Canonical:          s.canonical,
		HasHTTPS:           strings.HasPrefix(final, "https://"),
		MobileOK:           s.viewport,
		PerformanceScore:   intToF(performance),
		LighthouseSEOScore: intToF(lighthouseSEO),
		AccessibilityScore: intToF(accessibility),
		BestPracticesScore: intToF(bestPractices),
		WordCount:          len(words),
		QuestionHeadings:   questionHeadings,
		BusinessSchema:     businessSchema,
		OrganizationSchema: organizationSchema,
		SchemaTypes:        schemaTypes,
		SchemaFields:       schemaFields,
		ExternalLinks:      len(externalHosts),
		FactSignals:        factSignals,
		AuthorSignal:       s.metaAuthor,
		FreshSignal:        freshSignal,
		SemanticMain:       s.semanticMain,
		SocialCount:        len(s.socialLinks),
		ContactSignal:      s.tel || s.contact || s.forms > 0,
		TTFBMS:             ttfb,
	}
	vis := visibility.Calculate(evidence, 0)
	seoScore := vis.SEOScore

	weak := len(qualityFlags) >= 2 || !s.viewport || !strings.HasPrefix(final, "https://") ||
		(performance != nil && *performance < 45) || (seoScore != nil && *seoScore < 55)

	engine := "visibility+heuristic"
	if lighthouse != nil {
		engine = "lighthouse+visibility+heuristic"
		if usedPageSpeed {
			engine = "pagespeed+visibility+heuristic"
		}
	}
	result := Result{
		Reachable: true, Status: resp.StatusCode, FinalURL: final,
		HasHTTPS: strings.HasPrefix(final, "https://"), ResponseMS: elapsed, TTFBMS: ttfb,
		PerformanceScore: performance, AccessibilityScore: accessibility,
		BestPracticesScore: bestPractices, SEOScore: seoScore, LighthouseSEOScore: lighthouseSEO,
		AEOScore: vis.AEOScore, GEOScore: vis.GEOScore, AIVisibilityScore: vis.AIVisibilityScore,
		OpportunityGapScore: vis.OpportunityGapScore, ScoreConfidence: vis.ScoreConfidence,
		VisibilityReasons: vis.VisibilityReasons, VisibilitySignals: vis.VisibilitySignals,
		AuditEngine: engine, FieldData: fieldData, Indexable: evidence.Indexable, MobileOK: s.viewport,
		HasCTA: s.tel || s.contact || s.forms > 0, HasBooking: s.booking, Title: title,
		LatestCopyrightYear: latestYear, QualityFlags: qualityFlags, TrackingVendors: trackingVendors, SocialLinks: s.socialLinks,
		WebsiteStatus: "weak", Note: "Lighthouse scores are used when a local Lighthouse binary is available; otherwise PageSpeed Insights is used when enabled, and performance falls back to a TTFB proxy.",
	}
	if !weak {
		result.WebsiteStatus = "healthy"
	}
	if cache.Enabled() {
		cache.Put("audit", []any{normalized}, result)
	}
	return result, nil
}

func intToF(v *int) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}

func hostOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ = os.Getenv
