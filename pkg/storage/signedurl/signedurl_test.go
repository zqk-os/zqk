package signedurl

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestSigner_GenerateAndValidate(t *testing.T) {
	secret := os.Getenv(zqkenv.MCPExternalSecretKey())
	if secret == "" {
		secret = "fallback_for_tests"
	}
	signer := NewSigner(secret)

	path := "documents/report.pdf"
	ttl := 10 * time.Minute

	// Generate
	signedStr, err := signer.Generate(path, ttl)
	if err != nil {
		t.Fatalf("Failed to generate signed string: %v", err)
	}

	if signedStr == "" {
		t.Fatal("Expected signed string to not be empty")
	}

	// Validate
	validatedPath, err := signer.Validate(signedStr)
	if err != nil {
		t.Fatalf("Failed to validate signed string: %v", err)
	}

	if validatedPath != path {
		t.Errorf("Expected path %q, got %q", path, validatedPath)
	}
}

func TestSigner_ExpiredURL(t *testing.T) {
	secret := "secret"
	signer := NewSigner(secret)

	// Mock time
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	signer.now = func() time.Time { return baseTime }

	path := "test.txt"
	ttl := 5 * time.Minute

	signedStr, err := signer.Generate(path, ttl)
	if err != nil {
		t.Fatalf("Failed to generate: %v", err)
	}

	// Advance time past TTL
	signer.now = func() time.Time { return baseTime.Add(6 * time.Minute) }

	_, err = signer.Validate(signedStr)
	if !errors.Is(err, ErrExpired) {
		t.Errorf("Expected ErrExpired, got: %v", err)
	}
}

func TestSigner_TamperedPath(t *testing.T) {
	secret := "secret"
	signer := NewSigner(secret)

	path := "original.txt"
	ttl := 1 * time.Hour

	signedStr, err := signer.Generate(path, ttl)
	if err != nil {
		t.Fatalf("Failed to generate: %v", err)
	}

	// Tamper the path
	v, err := url.ParseQuery(signedStr)
	if err != nil {
		t.Fatal(err)
	}
	v.Set("path", "hacked.txt")
	tamperedStr := v.Encode()

	_, err = signer.Validate(tamperedStr)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Expected ErrInvalidSignature, got: %v", err)
	}
}

func TestSigner_TamperedSignature(t *testing.T) {
	secret := "secret"
	signer := NewSigner(secret)

	path := "file.txt"
	ttl := 1 * time.Hour

	signedStr, err := signer.Generate(path, ttl)
	if err != nil {
		t.Fatalf("Failed to generate: %v", err)
	}

	// Tamper the signature slightly
	tamperedStr := strings.Replace(signedStr, "sig=", "sig=00", 1)

	_, err = signer.Validate(tamperedStr)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Expected ErrInvalidSignature, got: %v", err)
	}
}

func TestSigner_InvalidFormat(t *testing.T) {
	signer := NewSigner("secret")

	invalidCases := []string{
		"path=hello",
		"path=hello&expires=123",
		"expires=123&sig=abc",
		"path=hello&expires=not_a_number&sig=abc",
		"::invalid_url::",
	}

	for _, tc := range invalidCases {
		_, err := signer.Validate(tc)
		if !errors.Is(err, ErrInvalidFormat) {
			t.Errorf("Expected ErrInvalidFormat for %q, got: %v", tc, err)
		}
	}
}
