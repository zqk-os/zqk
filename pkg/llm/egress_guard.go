package llm

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

var (
	// ErrEgressViolation indicates an unauthorized external network dispatch under zero-egress policy.
	ErrEgressViolation = errors.New("zero-egress policy violation: external egress blocked")
)

// EgressViolationError provides detailed context on a blocked network request.
type EgressViolationError struct {
	Code        string `json:"code"`
	TargetHost  string `json:"target_host"`
	TargetURL   string `json:"target_url"`
	Description string `json:"description"`
}

func (e *EgressViolationError) Error() string {
	return fmt.Sprintf("[%s] unauthorized egress to %s: %s (URL: %s)", e.Code, e.TargetHost, e.Description, e.TargetURL)
}

func (e *EgressViolationError) Unwrap() error {
	return ErrEgressViolation
}

// ZeroEgressGuard enforces network isolation by restricting outbound traffic to local loopback.
type ZeroEgressGuard struct {
	StrictLocalOnly bool
	AllowedHosts    map[string]bool
}

// NewZeroEgressGuard creates an initialized guard.
func NewZeroEgressGuard(strictLocalOnly bool) *ZeroEgressGuard {
	return &ZeroEgressGuard{
		StrictLocalOnly: strictLocalOnly,
		AllowedHosts: map[string]bool{
			"127.0.0.1": true,
			"::1":       true,
			"localhost": true,
		},
	}
}

// ValidateDestination verifies whether the destination URL is permitted.
func (g *ZeroEgressGuard) ValidateDestination(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return &EgressViolationError{
			Code:        "EGRESS_MALFORMED_ENDPOINT",
			TargetURL:   rawURL,
			Description: "destination URL cannot be empty",
		}
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return &EgressViolationError{
			Code:        "EGRESS_MALFORMED_ENDPOINT",
			TargetURL:   rawURL,
			Description: "destination URL must contain a valid scheme and host",
		}
	}

	host := u.Hostname()
	if host == "" {
		return &EgressViolationError{
			Code:        "EGRESS_MALFORMED_ENDPOINT",
			TargetURL:   rawURL,
			Description: "hostname could not be determined",
		}
	}

	if !g.StrictLocalOnly {
		return nil
	}

	// 1. Direct allowed hostnames
	if g.AllowedHosts[strings.ToLower(host)] {
		return nil
	}

	// 2. Loopback IP address check
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}

	// Fail closed
	return &EgressViolationError{
		Code:        "EGRESS_INTERCEPT_BLOCKED",
		TargetHost:  host,
		TargetURL:   rawURL,
		Description: "traffic directed outside loopback is prohibited under POL-LLM-ZERO-EGRESS-001",
	}
}

// RoundTripper wraps an http.RoundTripper with zero-egress enforcement.
type guardedRoundTripper struct {
	guard *ZeroEgressGuard
	next  http.RoundTripper
}

func (rt *guardedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, &EgressViolationError{
			Code:        "EGRESS_MALFORMED_ENDPOINT",
			Description: "nil http request or URL",
		}
	}

	if err := rt.guard.ValidateDestination(req.URL.String()); err != nil {
		return nil, err
	}

	next := rt.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(req)
}

// WrapTransport returns an http.RoundTripper protected by this guard.
func (g *ZeroEgressGuard) WrapTransport(next http.RoundTripper) http.RoundTripper {
	return &guardedRoundTripper{
		guard: g,
		next:  next,
	}
}
