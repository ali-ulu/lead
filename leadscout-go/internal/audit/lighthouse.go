package audit

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// LighthouseResult holds the category scores and any rendered DOM.
type LighthouseResult struct {
	Performance   *int
	Accessibility *int
	BestPractices *int
	SEO           *int
	RenderedHTML  string
}

func lighthouseCommand() []string {
	if configured := os.Getenv("LEADSCOUT_LIGHTHOUSE_BIN"); configured != "" {
		return []string{configured}
	}
	if bin, err := exec.LookPath("lighthouse"); err == nil {
		return []string{bin}
	}
	root := os.Getenv("LEADSCOUT_ROOT")
	if root == "" {
		root, _ = os.Getwd()
	}
	name := "lighthouse"
	if runtime.GOOS == "windows" {
		name = "lighthouse.cmd"
	}
	local := filepath.Join(root, "node_modules", ".bin", name)
	if _, err := os.Stat(local); err == nil {
		return []string{local}
	}
	if os.Getenv("LEADSCOUT_LIGHTHOUSE_AUTO_NPX") != "0" {
		if npx, err := exec.LookPath("npx"); err == nil {
			return []string{npx, "--yes", "lighthouse@13.5.0"}
		}
	}
	return nil
}

func runLighthouse(url string) *LighthouseResult {
	command := lighthouseCommand()
	if command == nil {
		return nil
	}
	args := append(command[1:],
		url, "--quiet", "--output=json", "--output-path=stdout",
		"--only-categories=performance,accessibility,best-practices,seo",
		"--chrome-flags=--headless --no-sandbox --disable-gpu",
	)
	cmd := exec.Command(command[0], args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &bytes.Buffer{}
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return nil
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-time.After(90 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return nil
	case err := <-done:
		if err != nil || stdout.Len() == 0 {
			return nil
		}
	}
	return parseLighthouse(stdout.Bytes())
}

func parseLighthouse(raw []byte) *LighthouseResult {
	var report struct {
		Categories map[string]struct {
			Score *float64 `json:"score"`
		} `json:"categories"`
		Audits map[string]struct {
			Details struct {
				HTML string `json:"html"`
			} `json:"details"`
		} `json:"audits"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil
	}
	score := func(name string) *int {
		c, ok := report.Categories[name]
		if !ok || c.Score == nil {
			return nil
		}
		v := int(*c.Score*100 + 0.5)
		return &v
	}
	result := &LighthouseResult{
		Performance:   score("performance"),
		Accessibility: score("accessibility"),
		BestPractices: score("best-practices"),
		SEO:           score("seo"),
	}
	for _, key := range []string{"final-screenshot", "full-page-screenshot"} {
		if a, ok := report.Audits[key]; ok && a.Details.HTML != "" {
			result.RenderedHTML = a.Details.HTML
			break
		}
	}
	return result
}
