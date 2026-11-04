// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package httpheaders

import (
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestConstantsAndStandardClassification(t *testing.T) {
	t.Parallel()

	// Verify constants match expected strings
	constants := map[string]string{
		Authorization:   "Authorization",
		ContentType:     "Content-Type",
		UserAgent:       "User-Agent",
		XAPIKey:         "X-API-Key",
		XEventType:      "X-Event-Type",
		XSource:         "X-Source",
		XTimestamp:      "X-Timestamp",
		Accept:          "Accept",
		AcceptEncoding:  "Accept-Encoding",
		CacheControl:    "Cache-Control",
		ContentLength:   "Content-Length",
		ETag:            "ETag",
		Host:            "Host",
		IfMatch:         "If-Match",
		IfNoneMatch:     "If-None-Match",
		Location:        "Location",
		Origin:          "Origin",
		RetryAfter:      "Retry-After",
		Traceparent:     "Traceparent",
		Tracestate:      "Tracestate",
		Forwarded:       "Forwarded",
		XForwardedFor:   "X-Forwarded-For",
		XForwardedHost:  "X-Forwarded-Host",
		XForwardedProto: "X-Forwarded-Proto",
		XRequestID:      "X-Request-Id",
		XCorrelationID:  "X-Correlation-Id",
	}

	for got, want := range constants {
		if got != want {
			t.Errorf("constant mismatch: got %q, want %q", got, want)
		}
	}

	// Verify IsStandard
	standardCases := []struct {
		header   string
		expected bool
	}{
		{ContentType, true},
		{"content-type", true},
		{Authorization, true},
		{"AUTHORIZATION", true},
		{Accept, true},
		{Traceparent, true},
		{Tracestate, true},
		{"X-Request-Id", false},
		{"X-Custom-Header", false},
		{"Unknown-Random-Header", false},
	}

	for _, tc := range standardCases {
		if got := IsStandard(tc.header); got != tc.expected {
			t.Errorf("IsStandard(%q) = %v; want %v", tc.header, got, tc.expected)
		}
	}

	// Verify IsCustom
	customCases := []struct {
		header   string
		expected bool
	}{
		{"X-Request-Id", true},
		{"x-request-id", true},
		{"X-API-Key", true},
		{"Zqk-Flow-Id", true},
		{"zqk-custom", true},
		{"Content-Type", false},
		{"Authorization", false},
		{"Unknown-Header", false},
	}

	for _, tc := range customCases {
		if got := IsCustom(tc.header); got != tc.expected {
			t.Errorf("IsCustom(%q) = %v; want %v", tc.header, got, tc.expected)
		}
	}
}

func TestCanonicalKeyAndMapOperations(t *testing.T) {
	t.Parallel()

	// CanonicalKey normalization
	keyCases := []struct {
		input string
		want  string
	}{
		{"content-type", "Content-Type"},
		{"CONTENT-TYPE", "Content-Type"},
		{"x-request-id", "X-Request-Id"},
		{"x-correlation-id", "X-Correlation-Id"},
		{"authorization", "Authorization"},
		{"accept-encoding", "Accept-Encoding"},
		{"sec-websocket-key", "Sec-Websocket-Key"},
	}

	for _, tc := range keyCases {
		if got := CanonicalKey(tc.input); got != tc.want {
			t.Errorf("CanonicalKey(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}

	// Nil Header operations (must be safe and not panic)
	var nilHeader http.Header
	if got := GetCanonical(nilHeader, ContentType); got != "" {
		t.Errorf("GetCanonical on nil header returned %q; want empty string", got)
	}
	if HasCanonical(nilHeader, ContentType) {
		t.Error("HasCanonical on nil header returned true; want false")
	}
	if got := Clone(nilHeader); got != nil {
		t.Errorf("Clone on nil header returned %v; want nil", got)
	}

	// Safe no-ops on nil header
	SetCanonical(nilHeader, ContentType, "text/plain")
	AddCanonical(nilHeader, ContentType, "text/html")
	DelCanonical(nilHeader, ContentType)

	// Populated Header operations
	h := make(http.Header)
	SetCanonical(h, "content-type", "application/json")
	if got := GetCanonical(h, "Content-Type"); got != "application/json" {
		t.Errorf("GetCanonical got %q; want application/json", got)
	}
	if !HasCanonical(h, "content-type") {
		t.Error("HasCanonical got false; want true")
	}

	// Add multiple values
	AddCanonical(h, "x-forwarded-for", "10.0.0.1")
	AddCanonical(h, "X-FORWARDED-FOR", "10.0.0.2")
	values := h["X-Forwarded-For"]
	if len(values) != 2 || values[0] != "10.0.0.1" || values[1] != "10.0.0.2" {
		t.Errorf("AddCanonical values mismatch: %v", values)
	}

	// DelCanonical
	DelCanonical(h, "Content-Type")
	if HasCanonical(h, "Content-Type") {
		t.Error("HasCanonical after DelCanonical got true; want false")
	}
	if got := GetCanonical(h, "content-type"); got != "" {
		t.Errorf("GetCanonical after Del got %q; want empty", got)
	}

	// Clone independence test
	cloned := Clone(h)
	if cloned == nil {
		t.Fatal("Clone returned nil for non-nil header")
	}
	if GetCanonical(cloned, "X-Forwarded-For") != "10.0.0.1" {
		t.Errorf("Cloned header missing value: %q", GetCanonical(cloned, "X-Forwarded-For"))
	}
	SetCanonical(cloned, "X-Forwarded-For", "192.168.1.1")
	if GetCanonical(h, "X-Forwarded-For") != "10.0.0.1" {
		t.Error("Modifying cloned header mutated original header")
	}
}

func TestValidation(t *testing.T) {
	t.Parallel()

	// ValidateHeaderName
	validNames := []string{
		"Content-Type",
		"X-Request-Id",
		"Authorization",
		"Accept-Encoding",
		"Custom_Field!#$%&'*+-.^_`|~",
		"A123",
	}
	for _, name := range validNames {
		if err := ValidateHeaderName(name); err != nil {
			t.Errorf("ValidateHeaderName(%q) unexpected error: %v", name, err)
		}
	}

	// Empty header name
	if err := ValidateHeaderName(""); !errors.Is(err, ErrEmptyHeaderName) {
		t.Errorf("ValidateHeaderName(\"\") got error %v; want ErrEmptyHeaderName", err)
	}

	// Invalid characters in header name
	invalidNames := []string{
		"Content Type",   // space
		"Content\tType",  // tab
		"Content\nType",  // newline
		"Content:Type",   // colon
		"Content/Type",   // slash
		"Content?Type",   // question mark
		"Content(Type)",  // parentheses
		"Content[Type]",  // brackets
		"Content{Type}",  // braces
		"Content<Type>",  // angle brackets
		"Content@Type",   // at sign
		"Content,Type",   // comma
		"Content;Type",   // semicolon
		"Content=Type",   // equals
		"Content\"Type",  // quote
		"Content\\Type",  // backslash
		"Header\x00Name", // null byte
		"Header\x7FName", // DEL
		"Header\x80Name", // non-ASCII byte
	}
	for _, name := range invalidNames {
		if err := ValidateHeaderName(name); !errors.Is(err, ErrInvalidHeaderNameChar) {
			t.Errorf("ValidateHeaderName(%q) got error %v; want ErrInvalidHeaderNameChar", name, err)
		}
	}

	// ValidateHeaderValue
	validValues := []string{
		"",
		"application/json",
		"Bearer token-12345",
		"text/plain; charset=utf-8",
		"val with spaces and \t tabs",
		"~*+-_.",
	}
	for _, val := range validValues {
		if err := ValidateHeaderValue(val); err != nil {
			t.Errorf("ValidateHeaderValue(%q) unexpected error: %v", val, err)
		}
	}

	// CRLF injection values
	crlfValues := []string{
		"malicious\r\nInjected-Header: evil",
		"value\rtest",
		"value\ntest",
	}
	for _, val := range crlfValues {
		if err := ValidateHeaderValue(val); !errors.Is(err, ErrCRLFInjection) {
			t.Errorf("ValidateHeaderValue(%q) got %v; want ErrCRLFInjection", val, err)
		}
	}

	// Null byte injection values
	if err := ValidateHeaderValue("evil\x00value"); !errors.Is(err, ErrNullByteInjection) {
		t.Errorf("ValidateHeaderValue with null byte got %v; want ErrNullByteInjection", err)
	}

	// Control characters in value
	controlCharValues := []string{
		"evil\x01value",
		"evil\x1Fvalue",
		"evil\x7Fvalue",
	}
	for _, val := range controlCharValues {
		if err := ValidateHeaderValue(val); !errors.Is(err, ErrInvalidHeaderValue) {
			t.Errorf("ValidateHeaderValue(%q) got %v; want ErrInvalidHeaderValue", val, err)
		}
	}

	// ValidateHeader combined
	if err := ValidateHeader("Content-Type", "application/json"); err != nil {
		t.Errorf("ValidateHeader unexpected error: %v", err)
	}
	if err := ValidateHeader("", "application/json"); !errors.Is(err, ErrEmptyHeaderName) {
		t.Errorf("ValidateHeader with empty name got %v; want ErrEmptyHeaderName", err)
	}
	if err := ValidateHeader("Content-Type", "invalid\r\nvalue"); !errors.Is(err, ErrCRLFInjection) {
		t.Errorf("ValidateHeader with CRLF value got %v; want ErrCRLFInjection", err)
	}
}

func TestAuthParsers(t *testing.T) {
	t.Parallel()

	// ExtractBearerToken tests
	t.Run("ExtractBearerToken", func(t *testing.T) {
		// Valid cases
		validCases := []struct {
			input string
			want  string
		}{
			{"Bearer test-token-123", "test-token-123"},
			{"bearer test-token-123", "test-token-123"},
			{"BEARER   test-token-123  ", "test-token-123"},
		}
		for _, tc := range validCases {
			got, err := ExtractBearerToken(tc.input)
			if err != nil {
				t.Errorf("ExtractBearerToken(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ExtractBearerToken(%q) = %q; want %q", tc.input, got, tc.want)
			}
		}

		// Empty auth header
		if _, err := ExtractBearerToken(""); !errors.Is(err, ErrMissingAuthHeader) {
			t.Errorf("ExtractBearerToken(\"\") got error %v; want ErrMissingAuthHeader", err)
		}
		if _, err := ExtractBearerToken("   "); !errors.Is(err, ErrMissingAuthHeader) {
			t.Errorf("ExtractBearerToken(\"   \") got error %v; want ErrMissingAuthHeader", err)
		}

		// Empty bearer payload
		if _, err := ExtractBearerToken("Bearer"); !errors.Is(err, ErrEmptyBearerToken) {
			t.Errorf("ExtractBearerToken(\"Bearer\") got error %v; want ErrEmptyBearerToken", err)
		}
		if _, err := ExtractBearerToken("Bearer   "); !errors.Is(err, ErrEmptyBearerToken) {
			t.Errorf("ExtractBearerToken(\"Bearer   \") got error %v; want ErrEmptyBearerToken", err)
		}

		// Invalid prefix
		if _, err := ExtractBearerToken("Basic dXNlcjpwYXNz"); !errors.Is(err, ErrInvalidBearerPrefix) {
			t.Errorf("ExtractBearerToken with Basic prefix got error %v; want ErrInvalidBearerPrefix", err)
		}
		if _, err := ExtractBearerToken("Token 12345"); !errors.Is(err, ErrInvalidBearerPrefix) {
			t.Errorf("ExtractBearerToken with Token prefix got error %v; want ErrInvalidBearerPrefix", err)
		}
		if _, err := ExtractBearerToken("short"); !errors.Is(err, ErrInvalidBearerPrefix) {
			t.Errorf("ExtractBearerToken(\"short\") got error %v; want ErrInvalidBearerPrefix", err)
		}
	})

	// ExtractBasicAuth tests
	t.Run("ExtractBasicAuth", func(t *testing.T) {
		// Valid case
		validCreds := base64.StdEncoding.EncodeToString([]byte("alice:secret123"))
		user, pass, ok := ExtractBasicAuth("Basic " + validCreds)
		if !ok || user != "alice" || pass != "secret123" {
			t.Errorf("ExtractBasicAuth failed: got %q, %q, %v", user, pass, ok)
		}

		// Case-insensitive scheme and colon in password
		complexCreds := base64.StdEncoding.EncodeToString([]byte("bob:p@ss:w0rd"))
		user, pass, ok = ExtractBasicAuth("basic " + complexCreds)
		if !ok || user != "bob" || pass != "p@ss:w0rd" {
			t.Errorf("ExtractBasicAuth with complex pass failed: got %q, %q, %v", user, pass, ok)
		}

		// Invalid scheme
		if _, _, ok := ExtractBasicAuth("Bearer token"); ok {
			t.Error("ExtractBasicAuth with Bearer returned ok = true")
		}
		if _, _, ok := ExtractBasicAuth("short"); ok {
			t.Error("ExtractBasicAuth with short input returned ok = true")
		}

		// Empty payload
		if _, _, ok := ExtractBasicAuth("Basic "); ok {
			t.Error("ExtractBasicAuth with empty payload returned ok = true")
		}

		// Invalid base64
		if _, _, ok := ExtractBasicAuth("Basic !!!notbase64!!!"); ok {
			t.Error("ExtractBasicAuth with corrupt base64 returned ok = true")
		}

		// Missing colon in decoded string
		noColonCreds := base64.StdEncoding.EncodeToString([]byte("alicepassword"))
		if _, _, ok := ExtractBasicAuth("Basic " + noColonCreds); ok {
			t.Error("ExtractBasicAuth with missing colon returned ok = true")
		}
	})

	// ExtractAPIKey tests
	t.Run("ExtractAPIKey", func(t *testing.T) {
		// Precedence: explicit xApiKeyHeader wins
		got := ExtractAPIKey("Bearer auth-key", "explicit-api-key")
		if got != "explicit-api-key" {
			t.Errorf("ExtractAPIKey precedence failed: got %q; want explicit-api-key", got)
		}

		// Fallbacks from Authorization header
		cases := []struct {
			authHeader string
			want       string
		}{
			{"ApiKey key-abc-123", "key-abc-123"},
			{"apikey key-abc-123", "key-abc-123"},
			{"Bearer key-abc-123", "key-abc-123"},
			{"Key key-abc-123", "key-abc-123"},
			{"raw-custom-key-value", "raw-custom-key-value"},
			{"Basic dXNlcjpwYXNz", ""}, // Basic auth excluded
			{"", ""},
			{"   ", ""},
		}

		for _, tc := range cases {
			if got := ExtractAPIKey(tc.authHeader, ""); got != tc.want {
				t.Errorf("ExtractAPIKey(%q, \"\") = %q; want %q", tc.authHeader, got, tc.want)
			}
		}
	})
}

func TestTraceContext(t *testing.T) {
	t.Parallel()

	// ExtractTraceContext with nil header
	tp, ts := ExtractTraceContext(nil)
	if tp != "" || ts != "" {
		t.Errorf("ExtractTraceContext(nil) = %q, %q; want empty strings", tp, ts)
	}

	// InjectTraceContext with nil header (safe no-op)
	InjectTraceContext(nil, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "congo=t61rcWkgMzE")

	// Injection and extraction lifecycle
	h := make(http.Header)
	wantTp := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	wantTs := "rojo=1,congo=2"

	InjectTraceContext(h, wantTp, wantTs)
	gotTp, gotTs := ExtractTraceContext(h)
	if gotTp != wantTp || gotTs != wantTs {
		t.Errorf("Trace context mismatch: got (%q, %q), want (%q, %q)", gotTp, gotTs, wantTp, wantTs)
	}

	// Partial injection (traceparent only)
	h2 := make(http.Header)
	InjectTraceContext(h2, wantTp, "")
	gotTp, gotTs = ExtractTraceContext(h2)
	if gotTp != wantTp || gotTs != "" {
		t.Errorf("Partial trace context mismatch: got (%q, %q); want (%q, \"\")", gotTp, gotTs, wantTp)
	}

	// Empty injection does not create empty headers
	h3 := make(http.Header)
	InjectTraceContext(h3, "", "   ")
	if len(h3) != 0 {
		t.Errorf("InjectTraceContext with whitespace created headers: %v", h3)
	}
}

func TestConcurrentOperations(t *testing.T) {
	t.Parallel()

	baseHeader := make(http.Header)
	SetCanonical(baseHeader, ContentType, "application/json")
	SetCanonical(baseHeader, Authorization, "Bearer secret-token-xyz")
	InjectTraceContext(baseHeader, "00-11112222333344445555666677778888-9999000011112222-01", "tenant=zqk")

	const workers = 16
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		workerID := i
		builder := goroutinelabels.NewGoroutine("httpheaders_worker", "concurrent header evaluation")
		builder.WithWaitGroup(&wg).StartSimple(func() {
			cloned := Clone(baseHeader)

			// Read and verify cloned values
			if GetCanonical(cloned, ContentType) != "application/json" {
				t.Errorf("worker %d: unexpected Content-Type", workerID)
			}
			token, err := ExtractBearerToken(GetCanonical(cloned, Authorization))
			if err != nil || token != "secret-token-xyz" {
				t.Errorf("worker %d: token extraction failed: %v", workerID, err)
			}
			tp, _ := ExtractTraceContext(cloned)
			if tp == "" {
				t.Errorf("worker %d: missing traceparent", workerID)
			}

			// Validate headers
			if err := ValidateHeader("X-Worker-Header", "worker-value"); err != nil {
				t.Errorf("worker %d: ValidateHeader failed: %v", workerID, err)
			}

			// Mutate cloned headers concurrently
			SetCanonical(cloned, XRequestID, "req-12345")
			DelCanonical(cloned, ContentType)
			if HasCanonical(cloned, ContentType) {
				t.Errorf("worker %d: ContentType still present after delete", workerID)
			}
		})
	}

	wg.Wait()

	// Invariant: original baseHeader was never modified by concurrent workers
	if GetCanonical(baseHeader, ContentType) != "application/json" {
		t.Error("baseHeader was mutated during concurrent operations")
	}
	if HasCanonical(baseHeader, XRequestID) {
		t.Error("baseHeader unexpectedly contains worker X-Request-Id")
	}
}
