package io

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Throttler provides thread-safe per-domain rate limiting.
type Throttler struct {
	mu              sync.Mutex
	domains         map[string]time.Time
	domainIntervals map[string]time.Duration
	defaultInterval time.Duration
}

// NewThrottler creates a new domain throttler with the specified default interval between requests.
func NewThrottler(defaultInterval time.Duration) *Throttler {
	if defaultInterval <= 0 {
		defaultInterval = 500 * time.Millisecond
	}
	return &Throttler{
		domains:         make(map[string]time.Time),
		domainIntervals: make(map[string]time.Duration),
		defaultInterval: defaultInterval,
	}
}

// DefaultThrottler is the system-wide default throttler instance.
var DefaultThrottler = NewThrottler(500 * time.Millisecond)

// SetDomainInterval configures a custom minimum interval for a specific domain.
func (t *Throttler) SetDomainInterval(domain string, interval time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain != "" {
		t.domainIntervals[domain] = interval
	}
}

// SetDefaultInterval updates the default interval for unspecified domains.
func (t *Throttler) SetDefaultInterval(interval time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if interval > 0 {
		t.defaultInterval = interval
	}
}

// ExtractDomain parses the hostname domain from a target URL.
func ExtractDomain(rawURL string) (string, error) {
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", errfmt.Newf("invalid target URL %q: %v", rawURL, err)
	}
	host := u.Hostname()
	if host == "" {
		return "", errfmt.Errorf("could not extract domain from URL %q", rawURL)
	}
	return strings.ToLower(host), nil
}

// Wait blocks until the per-domain throttle allows making a request to targetURL.
func (t *Throttler) Wait(ctx context.Context, targetURL string) error {
	domain, err := ExtractDomain(targetURL)
	if err != nil {
		return err
	}
	return t.WaitDomain(ctx, domain)
}

// WaitDomain blocks until the per-domain throttle allows making a request to domain.
func (t *Throttler) WaitDomain(ctx context.Context, domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return errfmt.Errorf("empty domain provided to throttler")
	}

	var waitDuration time.Duration

	t.mu.Lock()
	interval := t.defaultInterval
	if custom, ok := t.domainIntervals[domain]; ok {
		interval = custom
	}

	lastAccess, exists := t.domains[domain]
	now := time.Now()
	if exists {
		elapsed := now.Sub(lastAccess)
		if elapsed < interval {
			waitDuration = interval - elapsed
		}
	}

	// Update last access time to the planned execution time
	t.domains[domain] = now.Add(waitDuration)
	t.mu.Unlock()

	if waitDuration > 0 {
		select {
		case <-time.After(waitDuration):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// GetLastAccess returns the recorded timestamp for domain.
func (t *Throttler) GetLastAccess(domain string) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.domains[strings.ToLower(strings.TrimSpace(domain))]
}

// Reset clears all recorded domain timestamps.
func (t *Throttler) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.domains = make(map[string]time.Time)
}
