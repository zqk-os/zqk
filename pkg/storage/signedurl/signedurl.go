package signedurl

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

var (
	// ErrInvalidSignature indicates the signature does not match the payload.
	ErrInvalidSignature = errors.New("invalid signature")
	// ErrExpired indicates the URL's TTL has passed.
	ErrExpired = errors.New("url has expired")
	// ErrInvalidFormat indicates the provided string is not a valid signed URL.
	ErrInvalidFormat = errors.New("invalid url format")
)

// Signer handles generating and validating signed URLs.
type Signer struct {
	secret []byte
	now    func() time.Time // useful for testing
}

// NewSigner creates a new Signer with the provided secret.
func NewSigner(secret string) *Signer {
	return &Signer{
		secret: []byte(secret),
		now:    time.Now,
	}
}

// Generate creates a cryptographically signed string including the path, expiration, and signature.
func (s *Signer) Generate(path string, ttl time.Duration) (string, error) {
	expires := s.now().Add(ttl).Unix()

	// The payload contains the path and expiration time.
	payload := fmt.Sprintf("%s:%d", path, expires)

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))

	v := url.Values{}
	v.Set("path", path)
	v.Set("expires", strconv.FormatInt(expires, 10))
	v.Set("sig", signature)

	return v.Encode(), nil
}

// Validate verifies a signed string, checking format, signature, and expiration.
// It returns the original path if successful.
func (s *Signer) Validate(signedString string) (string, error) {
	v, err := url.ParseQuery(signedString)
	if err != nil {
		return "", ErrInvalidFormat
	}

	path := v.Get("path")
	expiresStr := v.Get("expires")
	sig := v.Get("sig")

	if path == "" || expiresStr == "" || sig == "" {
		return "", ErrInvalidFormat
	}

	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return "", ErrInvalidFormat
	}

	// Verify the signature first to prevent timing-based analysis of expiration logic
	payload := fmt.Sprintf("%s:%d", path, expires)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return "", ErrInvalidSignature
	}

	// Check expiration
	if s.now().Unix() > expires {
		return "", ErrExpired
	}

	return path, nil
}
