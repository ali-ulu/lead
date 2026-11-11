// Package ratelimit adds per-key token-bucket spacing, bounded retries with
// backoff, and a circuit breaker so one dead provider cannot stall a scan.
package ratelimit

import (
	"errors"
	"os"
	"strconv"
	"sync"
	"time"
)

type bucket struct {
	minInterval float64
	next        float64
}

type breaker struct {
	threshold int
	cooldown  float64
	failures  int
	openUntil float64
}

var (
	mu       sync.Mutex
	buckets  = map[string]*bucket{}
	breakers = map[string]*breaker{}
)

// CircuitOpen is returned when a provider is skipped because its breaker is open.
var CircuitOpen = errors.New("provider temporarily unavailable after repeated failures")

func intEnv(name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return int(f)
}

func now() float64 { return float64(time.Now().UnixNano()) / 1e9 }

func bucketFor(key string) *bucket {
	if b, ok := buckets[key]; ok {
		return b
	}
	b := &bucket{}
	buckets[key] = b
	return b
}

func breakerFor(key string) *breaker {
	if b, ok := breakers[key]; ok {
		return b
	}
	b := &breaker{
		threshold: intEnv("LEADSCOUT_CIRCUIT_THRESHOLD", 4),
		cooldown:  float64(intEnv("LEADSCOUT_CIRCUIT_COOLDOWN", 60)),
	}
	breakers[key] = b
	return b
}

// Configure adjusts defaults for a provider key (used by tests).
func Configure(key string, minInterval float64, failureThreshold int, cooldown float64) {
	mu.Lock()
	defer mu.Unlock()
	b := bucketFor(key)
	if minInterval > 0 {
		b.minInterval = minInterval
	}
	br := breakerFor(key)
	if failureThreshold > 0 {
		br.threshold = failureThreshold
	}
	if cooldown > 0 {
		br.cooldown = cooldown
	}
}

// Reset clears all buckets and breakers.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	buckets = map[string]*bucket{}
	breakers = map[string]*breaker{}
}

func waitBucket(key string, minInterval float64) {
	mu.Lock()
	b := bucketFor(key)
	if minInterval > b.minInterval {
		b.minInterval = minInterval
	}
	interval := b.minInterval
	n := b.next
	cur := now()
	if cur < n {
		mu.Unlock()
		time.Sleep(time.Duration((n - cur) * float64(time.Second)))
		mu.Lock()
		cur = now()
	}
	b.next = cur + interval
	mu.Unlock()
}

func allow(key string) bool {
	mu.Lock()
	defer mu.Unlock()
	return now() >= breakerFor(key).openUntil
}

func recordSuccess(key string) {
	mu.Lock()
	defer mu.Unlock()
	b := breakerFor(key)
	b.failures = 0
	b.openUntil = 0
}

func recordFailure(key string) {
	mu.Lock()
	defer mu.Unlock()
	b := breakerFor(key)
	b.failures++
	if b.threshold > 0 && b.failures >= b.threshold {
		b.openUntil = now() + b.cooldown
	}
}

// Options controls a Call.
type Options struct {
	MinInterval    float64
	Retries        int
	Backoff        float64
	DisableBreaker bool
}

// Call wraps fn with spacing, retries and circuit breaking.
func Call[T any](key string, fn func() (T, error), opts Options) (T, error) {
	var zero T
	if opts.MinInterval == 0 {
		opts.MinInterval = float64(intEnv("LEADSCOUT_MIN_INTERVAL_MS", 0)) / 1000.0
	}
	if opts.Backoff == 0 {
		opts.Backoff = 0.5
	}
	if !opts.DisableBreaker && !allow(key) {
		return zero, CircuitOpen
	}

	waitBucket(key, opts.MinInterval)

	var lastErr error
	for attempt := 0; attempt <= opts.Retries; attempt++ {
		value, err := fn()
		if err == nil {
			if !opts.DisableBreaker {
				recordSuccess(key)
			}
			return value, nil
		}
		lastErr = err
		if isTransient(err) && attempt < opts.Retries {
			time.Sleep(time.Duration(opts.Backoff * float64(int64(1)<<attempt) * float64(time.Second)))
			continue
		}
		if !opts.DisableBreaker {
			recordFailure(key)
		}
		return zero, err
	}
	return zero, lastErr
}

type netError interface{ Timeout() bool }

func isTransient(err error) bool {
	var ne netError
	if errors.As(err, &ne) {
		return true
	}
	// Mirror Python: OSError/TimeoutError are transient. Go network errors
	// satisfy net.Error; treat anything else as permanent.
	var te interface{ Temporary() bool }
	return errors.As(err, &te)
}
