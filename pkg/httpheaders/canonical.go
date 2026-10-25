// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package httpheaders

import (
	"net/http"
	"net/textproto"
)

// CanonicalKey returns the canonical MIME header key format for s.
// In canonical format, the first letter and any letter following a hyphen
// are uppercase; the rest are lowercase (RFC 7230/9110).
func CanonicalKey(s string) string {
	return textproto.CanonicalMIMEHeaderKey(s)
}

// GetCanonical retrieves the first value associated with the given key from the Header map
// after canonicalizing the key. If h is nil or key is not present, it returns an empty string.
func GetCanonical(h http.Header, key string) string {
	if h == nil {
		return ""
	}
	return h.Get(CanonicalKey(key))
}

// SetCanonical sets the header entries associated with key to the single element val,
// using the canonical representation of key. If h is nil, SetCanonical does nothing.
func SetCanonical(h http.Header, key, val string) {
	if h == nil {
		return
	}
	h.Set(CanonicalKey(key), val)
}

// AddCanonical adds the key/val pair to the header map using the canonical representation of key.
// If h is nil, AddCanonical does nothing.
func AddCanonical(h http.Header, key, val string) {
	if h == nil {
		return
	}
	h.Add(CanonicalKey(key), val)
}

// DelCanonical deletes the values associated with key using the canonical representation of key.
// If h is nil, DelCanonical does nothing.
func DelCanonical(h http.Header, key string) {
	if h == nil {
		return
	}
	h.Del(CanonicalKey(key))
}

// HasCanonical reports whether the header map contains any value for key.
// If h is nil, HasCanonical returns false.
func HasCanonical(h http.Header, key string) bool {
	if h == nil {
		return false
	}
	_, ok := h[CanonicalKey(key)]
	return ok
}

// Clone returns a deep copy of the provided http.Header.
// If h is nil, Clone returns nil.
func Clone(h http.Header) http.Header {
	if h == nil {
		return nil
	}
	return h.Clone()
}
