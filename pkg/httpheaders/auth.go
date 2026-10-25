// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package httpheaders

import (
	"encoding/base64"
	"errors"
	"strings"
)

var (
	// ErrMissingAuthHeader indicates the Authorization header is empty or missing.
	ErrMissingAuthHeader = errors.New("httpheaders: authorization header is empty")

	// ErrInvalidBearerPrefix indicates the header does not begin with the Bearer scheme.
	ErrInvalidBearerPrefix = errors.New("httpheaders: authorization header does not have Bearer prefix")

	// ErrEmptyBearerToken indicates the Bearer token payload is missing or empty.
	ErrEmptyBearerToken = errors.New("httpheaders: bearer token is empty")
)

// ExtractBearerToken parses an Authorization header value and extracts the Bearer token.
// The scheme matching is case-insensitive. Returns an error if the header is empty,
// does not start with "Bearer ", or has an empty token payload.
func ExtractBearerToken(authHeader string) (string, error) {
	trimmed := strings.TrimSpace(authHeader)
	if trimmed == "" {
		return "", ErrMissingAuthHeader
	}
	if strings.EqualFold(trimmed, "bearer") {
		return "", ErrEmptyBearerToken
	}
	if len(trimmed) < 7 || !strings.EqualFold(trimmed[:7], "bearer ") {
		return "", ErrInvalidBearerPrefix
	}
	token := strings.TrimSpace(trimmed[7:])
	if token == "" {
		return "", ErrEmptyBearerToken
	}
	return token, nil
}

// ExtractBasicAuth parses an Authorization header value in Basic authentication format (RFC 7617).
// It decodes the base64-encoded credentials and returns the username and password.
// Returns ok = false if the header is not a valid Basic auth header or base64 decoding fails.
func ExtractBasicAuth(authHeader string) (username, password string, ok bool) {
	trimmed := strings.TrimSpace(authHeader)
	if len(trimmed) < 6 || !strings.EqualFold(trimmed[:6], "basic ") {
		return "", "", false
	}
	encoded := strings.TrimSpace(trimmed[6:])
	if encoded == "" {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", false
	}
	user, pass, found := strings.Cut(string(decoded), ":")
	if !found {
		return "", "", false
	}
	return user, pass, true
}

// ExtractAPIKey resolves an API key from either the explicit X-API-Key header
// or an Authorization header fallback (supporting "ApiKey <key>", "Bearer <key>", "Key <key>", or raw value).
// An explicit xApiKeyHeader always takes precedence.
func ExtractAPIKey(authHeader, xApiKeyHeader string) string {
	if k := strings.TrimSpace(xApiKeyHeader); k != "" {
		return k
	}
	auth := strings.TrimSpace(authHeader)
	if auth == "" {
		return ""
	}
	schemes := []string{"apikey ", "bearer ", "key "}
	for _, scheme := range schemes {
		if len(auth) > len(scheme) && strings.EqualFold(auth[:len(scheme)], scheme) {
			token := strings.TrimSpace(auth[len(scheme):])
			if token != "" {
				return token
			}
		}
	}
	// Basic auth is not an API key
	if len(auth) >= 6 && strings.EqualFold(auth[:6], "basic ") {
		return ""
	}
	return auth
}
