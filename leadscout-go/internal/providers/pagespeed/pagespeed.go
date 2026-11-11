// Package pagespeed calls Google's PageSpeed Insights API.
//
// Lighthouse needs a local headless Chrome and takes tens of seconds. The
// PageSpeed Insights API runs the same audit remotely and also returns CrUX
// field data (real-user metrics), so it is a good fallback when Chrome is
// unavailable or when field data is wanted.
//
// The anonymous quota is tiny; set GOOGLE_PSI_API_KEY for a larger one.
package pagespeed

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/netguard"
)

// Endpoint is the PageSpeed Insights v5 runPagespeed method. A variable so
// tests can point it at a local server.
var Endpoint = "https://www.googleapis.com/pagespeedonline/v5/runPagespeed"

// Categories requested from the API.
var Categories = []string{"performance", "accessibility", "best-practices", "seo"}

// Result is the mapped audit outcome.
type Result struct {
	Performance   *int           `json:"performance_score"`
	Accessibility *int           `json:"accessibility_score"`
	BestPractices *int           `json:"best_practices_score"`
	SEO           *int           `json:"seo_score"`
	LighthouseVer string         `json:"lighthouse_version,omitempty"`
	FieldData     map[string]any `json:"field_data,omitempty"`
	Source        string         `json:"source"`
}

// Configured reports whether the fallback may run. It is opt-in because the
// anonymous quota is small and shared.
func Configured() bool {
	if strings.TrimSpace(os.Getenv("GOOGLE_PSI_API_KEY")) != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv("LEADSCOUT_PAGESPEED")) == "1"
}

type category struct {
	Score *float64 `json:"score"`
}

type metric struct {
	Percentile *float64 `json:"percentile"`
	Category   string   `json:"category"`
}

type experience struct {
	OverallCategory string            `json:"overall_category"`
	Metrics         map[string]metric `json:"metrics"`
}

type response struct {
	LoadingExperience       experience `json:"loadingExperience"`
	OriginLoadingExperience experience `json:"originLoadingExperience"`
	LighthouseResult        struct {
		LighthouseVersion string              `json:"lighthouseVersion"`
		Categories        map[string]category `json:"categories"`
	} `json:"lighthouseResult"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func score(categories map[string]category, name string) *int {
	c, ok := categories[name]
	if !ok || c.Score == nil {
		return nil
	}
	v := int(*c.Score*100 + 0.5)
	return &v
}

func fieldData(payload response) map[string]any {
	exp := payload.LoadingExperience
	if len(exp.Metrics) == 0 {
		exp = payload.OriginLoadingExperience
	}
	if len(exp.Metrics) == 0 {
		return nil
	}
	out := map[string]any{}
	for key, value := range exp.Metrics {
		out[key] = map[string]any{"percentile": value.Percentile, "category": value.Category}
	}
	if exp.OverallCategory != "" {
		out["overall_category"] = exp.OverallCategory
	}
	return out
}

// ParseResponse maps a runPagespeed payload into the audit score shape.
func ParseResponse(raw []byte) (*Result, error) {
	var payload response
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload.Error != nil {
		message := payload.Error.Message
		if message == "" {
			message = "PageSpeed Insights returned an error."
		}
		return nil, fmt.Errorf("%s", message)
	}
	cats := payload.LighthouseResult.Categories
	return &Result{
		Performance:   score(cats, "performance"),
		Accessibility: score(cats, "accessibility"),
		BestPractices: score(cats, "best-practices"),
		SEO:           score(cats, "seo"),
		LighthouseVer: payload.LighthouseResult.LighthouseVersion,
		FieldData:     fieldData(payload),
		Source:        "pagespeed",
	}, nil
}

// BuildURL assembles the runPagespeed request URL. Exposed for testing.
func BuildURL(rawURL, strategy string) string {
	if strategy == "" {
		strategy = "mobile"
	}
	values := url.Values{}
	values.Set("url", rawURL)
	values.Set("strategy", strategy)
	for _, c := range Categories {
		values.Add("category", c)
	}
	if key := strings.TrimSpace(os.Getenv("GOOGLE_PSI_API_KEY")); key != "" {
		values.Set("key", key)
	}
	return Endpoint + "?" + values.Encode()
}

// Analyze runs PageSpeed Insights for rawURL and returns the mapped scores.
func Analyze(rawURL string, strategy string, timeout time.Duration) (*Result, error) {
	if !Configured() {
		return nil, fmt.Errorf("PageSpeed Insights is disabled (set LEADSCOUT_PAGESPEED=1 or GOOGLE_PSI_API_KEY)")
	}
	requestURL := BuildURL(rawURL, strategy)
	if err := netguard.AssertPublicURL(requestURL); err != nil {
		return nil, err
	}
	client := netguard.NewPublicClient(timeout, false)
	req, _ := http.NewRequest(http.MethodGet, requestURL, nil)
	req.Header.Set("User-Agent", netguard.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4_000_000))
	return ParseResponse(body)
}
