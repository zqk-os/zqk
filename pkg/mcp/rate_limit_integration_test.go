// Package mcp: tests for rate limit config and behavior (BLI-645).
package mcp

import (
	"testing"
)

func TestApplyRateLimitConfig_enabled(t *testing.T) {
	s := NewServer()
	config := &ServerConfig{}
	config.MCPServer.RateLimit.Enabled = true
	config.MCPServer.RateLimit.RequestsPerMinute = 2

	b := NewServerLifecycleBuilder(s)
	b.config = config
	b.ApplyRateLimitConfig()

	if s.rateLimiter == nil {
		t.Fatal("expected rate limiter when enabled")
	}

	key := s.getRateLimitKey(false)
	if !s.rateLimiter.Allow(key) {
		t.Error("first request should be allowed")
	}
	if !s.rateLimiter.Allow(key) {
		t.Error("second request should be allowed")
	}
	if s.rateLimiter.Allow(key) {
		t.Error("third request should be denied")
	}
}

func TestApplyRateLimitConfig_disabled(t *testing.T) {
	s := NewServer()
	config := &ServerConfig{}
	config.MCPServer.RateLimit.Enabled = false

	b := NewServerLifecycleBuilder(s)
	b.config = config
	b.ApplyRateLimitConfig()

	if s.rateLimiter != nil {
		t.Error("expected nil rate limiter when disabled")
	}
}

func TestApplyRateLimitConfig_nilConfig(t *testing.T) {
	s := NewServer()
	b := NewServerLifecycleBuilder(s)
	b.config = nil
	b.ApplyRateLimitConfig()

	if s.rateLimiter != nil {
		t.Error("expected nil rate limiter when config is nil")
	}
}

func TestGetRateLimitKey_global(t *testing.T) {
	s := NewServer()
	if key := s.getRateLimitKey(false); key != "global" {
		t.Errorf("getRateLimitKey(false) = %q, want global", key)
	}
	s.secCtx = nil
	if key := s.getRateLimitKey(true); key != "global" {
		t.Errorf("getRateLimitKey(true) with nil secCtx = %q, want global", key)
	}
}
