// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package httpheaders

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrEmptyHeaderName indicates a header name is empty.
	ErrEmptyHeaderName = errors.New("httpheaders: header name cannot be empty")

	// ErrInvalidHeaderNameChar indicates a header name contains forbidden characters.
	ErrInvalidHeaderNameChar = errors.New("httpheaders: header name contains invalid token character")

	// ErrCRLFInjection indicates an attempt at CRLF header injection.
	ErrCRLFInjection = errors.New("httpheaders: header value contains forbidden CRLF characters")

	// ErrNullByteInjection indicates an attempt to inject null bytes into a header value.
	ErrNullByteInjection = errors.New("httpheaders: header value contains forbidden null byte")

	// ErrInvalidHeaderValue indicates a header value contains forbidden control characters.
	ErrInvalidHeaderValue = errors.New("httpheaders: header value contains invalid control character")
)

// isTokenChar reports whether c is a valid token character according to RFC 7230 section 3.2.6.
// token = 1*tchar
// tchar = "!" / "#" / "$" / "%" / "&" / "'" / "*" / "+" / "-" / "." / "^" / "_" / "`" / "|" / "~" / DIGIT / ALPHA
func isTokenChar(c byte) bool {
	if c >= 'a' && c <= 'z' {
		return true
	}
	if c >= 'A' && c <= 'Z' {
		return true
	}
	if c >= '0' && c <= '9' {
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}

// ValidateHeaderName validates that name conforms to RFC 7230/9110 token requirements.
// It verifies that name is non-empty and contains only valid token characters (no whitespace,
// control characters, colons, or separators).
func ValidateHeaderName(name string) error {
	if len(name) == 0 {
		return ErrEmptyHeaderName
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !isTokenChar(c) {
			return fmt.Errorf("%w: byte 0x%02x (%q) at index %d in %q", ErrInvalidHeaderNameChar, c, c, i, name)
		}
	}
	return nil
}

// ValidateHeaderValue inspects value for security risks including CRLF injection (\r, \n),
// null byte injection (\x00), and invalid non-printable control characters.
// HTAB (0x09) and space (0x20) are permitted as legitimate field value whitespace.
func ValidateHeaderValue(value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%w: %q", ErrCRLFInjection, value)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%w: %q", ErrNullByteInjection, value)
	}
	for i := 0; i < len(value); i++ {
		b := value[i]
		if (b < 0x20 && b != 0x09) || b == 0x7F {
			return fmt.Errorf("%w: byte 0x%02x at index %d in %q", ErrInvalidHeaderValue, b, i, value)
		}
	}
	return nil
}

// ValidateHeader validates both the header field name and its corresponding value.
func ValidateHeader(name, value string) error {
	if err := ValidateHeaderName(name); err != nil {
		return err
	}
	return ValidateHeaderValue(value)
}
