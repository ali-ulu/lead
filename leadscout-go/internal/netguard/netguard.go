// Package netguard provides outbound-network guards: SSRF protection,
// robots.txt checks and rate limiting. Enrichment and auditing fetch URLs from
// untrusted third-party data, so every fetch re-validates the host and each
// redirect hop, and re-checks the peer address actually connected to.
package netguard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UserAgent identifies this scanner to the sites and providers it contacts.
const UserAgent = "LeadScout/7.0 (+https://github.com/ali-ulu/lead; polite local scanner)"

var privateHostnames = map[string]bool{
	"localhost": true, "localhost.localdomain": true, "ip6-localhost": true,
	"ip6-loopback": true, "metadata": true, "metadata.google.internal": true,
}

// lookupIP resolves a hostname; overridable in tests.
var lookupIP = net.LookupIP

func isPublicIP(ip net.IP) bool {
	return !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() ||
		isReserved(ip))
}

func isReserved(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		// 240.0.0.0/4 reserved
		return ip4[0] >= 240
	}
	return false
}

// ErrUnresolved marks a hostname that does not resolve (a dead domain).
var ErrUnresolved = errors.New("Website hostname could not be resolved")

// ErrPrivate marks a URL that resolves to a local/private address.
var ErrPrivate = errors.New("Local/private websites are not audited")

// AssertPublicURL rejects URLs that resolve to a local, private or reserved
// address. A DNS failure returns ErrUnresolved so callers can treat a dead
// domain as a finding rather than an error.
func AssertPublicURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("Invalid website hostname")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("Invalid website hostname")
	}
	if privateHostnames[strings.ToLower(host)] {
		return ErrPrivate
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	ips, err := lookupIP(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("%w: %v", ErrUnresolved, err)
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return ErrPrivate
		}
	}
	return nil
}

// IsUnresolved reports whether err is (or wraps) ErrUnresolved.
func IsUnresolved(err error) bool { return errors.Is(err, ErrUnresolved) }

// NewPublicClient returns an HTTP client that re-validates every redirect hop
// and the peer address of the connection it opens.
func NewPublicClient(timeout time.Duration, checkRobots bool) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			peer, ok := conn.RemoteAddr().(*net.TCPAddr)
			if ok && !isPublicIP(peer.IP) {
				_ = conn.Close()
				return nil, ErrPrivate
			}
			return conn, nil
		},
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if err := AssertPublicURL(req.URL.String()); err != nil {
			return err
		}
		if checkRobots && RespectRobots() && !RobotsAllows(req.URL.String(), 5*time.Second) {
			return errors.New("Blocked by robots.txt")
		}
		return nil
	}
	return client
}

// RespectRobots reports whether robots.txt is honoured (default on).
func RespectRobots() bool {
	return os.Getenv("LEADSCOUT_RESPECT_ROBOTS") != "0"
}

// --- robots.txt -----------------------------------------------------------

type robotsEntry struct {
	at        time.Time
	disallows []string
}

var (
	robotsMu    sync.Mutex
	robotsCache = map[string]robotsEntry{}
)

const robotsTTL = time.Hour

func parseRobots(body string) []string {
	type block struct {
		ua    string
		rules []string
	}
	var blocks []block
	var current *block
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		field := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		switch field {
		case "user-agent":
			blocks = append(blocks, block{ua: value})
			current = &blocks[len(blocks)-1]
		case "disallow":
			if current != nil {
				current.rules = append(current.rules, value)
			}
		}
	}
	var specific, star [][]string
	for i := range blocks {
		b := blocks[i]
		if b.ua != "*" && strings.Contains(strings.ToLower(b.ua), "leadscout") {
			specific = append(specific, b.rules)
		} else if b.ua == "*" {
			star = append(star, b.rules)
		}
	}
	chosen := specific
	if len(chosen) == 0 {
		chosen = star
	}
	var disallows []string
	for _, rules := range chosen {
		for _, r := range rules {
			if r != "" {
				disallows = append(disallows, r)
			}
		}
	}
	return disallows
}

func fetchRobots(origin string, timeout time.Duration) []string {
	robotsURL := origin + "/robots.txt"
	if err := AssertPublicURL(robotsURL); err != nil {
		return nil
	}
	client := NewPublicClient(timeout, false)
	resp, err := client.Get(robotsURL)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	body := make([]byte, 0, 4096)
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if err != nil || len(body) > 512_000 {
			break
		}
	}
	return parseRobots(string(body))
}

// RobotsAllows performs a best-effort robots.txt check; unknown allows.
func RobotsAllows(rawURL string, timeout time.Duration) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return true
	}
	origin := u.Scheme + "://" + u.Host
	robotsMu.Lock()
	entry, ok := robotsCache[origin]
	robotsMu.Unlock()
	var disallows []string
	if ok && time.Since(entry.at) < robotsTTL {
		disallows = entry.disallows
	} else {
		disallows = fetchRobots(origin, timeout)
		robotsMu.Lock()
		robotsCache[origin] = robotsEntry{at: time.Now(), disallows: disallows}
		robotsMu.Unlock()
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path = path + "?" + u.RawQuery
	}
	for _, rule := range disallows {
		if rule == "/" {
			return false
		}
		if strings.HasSuffix(rule, "*") && strings.HasPrefix(path, strings.TrimSuffix(rule, "*")) {
			return false
		}
		if strings.HasPrefix(path, rule) {
			return false
		}
	}
	return true
}

// --- rate limiting --------------------------------------------------------

// Limiter enforces a minimum interval between calls.
type Limiter struct {
	mu          sync.Mutex
	minInterval time.Duration
	next        time.Time
}

// NewLimiter builds a limiter with the given minimum interval.
func NewLimiter(minInterval time.Duration) *Limiter {
	return &Limiter{minInterval: minInterval}
}

// Wait blocks until the next slot is available.
func (l *Limiter) Wait() {
	if l.minInterval <= 0 {
		return
	}
	l.mu.Lock()
	nowT := time.Now()
	if nowT.Before(l.next) {
		d := l.next.Sub(nowT)
		l.mu.Unlock()
		time.Sleep(d)
		l.mu.Lock()
		nowT = time.Now()
	}
	l.next = nowT.Add(l.minInterval)
	l.mu.Unlock()
}

func envFloat(name string, def float64) float64 {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return f
}

// DefaultAuditInterval returns LEADSCOUT_AUDIT_MIN_INTERVAL seconds.
func DefaultAuditInterval() time.Duration {
	return time.Duration(envFloat("LEADSCOUT_AUDIT_MIN_INTERVAL", 0) * float64(time.Second))
}

// DefaultFetchInterval returns LEADSCOUT_FETCH_MIN_INTERVAL seconds.
func DefaultFetchInterval() time.Duration {
	return time.Duration(envFloat("LEADSCOUT_FETCH_MIN_INTERVAL", 0) * float64(time.Second))
}

// HostOf returns the hostname of a URL (or the input when unparseable).
func HostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return rawURL
	}
	return u.Hostname()
}
