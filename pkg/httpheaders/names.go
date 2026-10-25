// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package httpheaders

import "strings"

// Common HTTP header names (avoid repeating string literals at Set/Add/Del call sites).
const (
	Authorization = "Authorization"
	ContentType   = "Content-Type"
	UserAgent     = "User-Agent"
	XAPIKey       = "X-API-Key" //nolint:gosec
	XEventType    = "X-Event-Type"
	XSource       = "X-Source"
	XTimestamp    = "X-Timestamp"

	// Standard RFC 7231 / 9110 HTTP headers.
	Accept         = "Accept"
	AcceptEncoding = "Accept-Encoding"
	CacheControl   = "Cache-Control"
	ContentLength  = "Content-Length"
	ETag           = "ETag"
	Host           = "Host"
	IfMatch        = "If-Match"
	IfNoneMatch    = "If-None-Match"
	Location       = "Location"
	Origin         = "Origin"
	RetryAfter     = "Retry-After"

	// W3C Trace Context headers (RFC / W3C Recommendation).
	Traceparent = "Traceparent"
	Tracestate  = "Tracestate"

	// Proxy and routing headers.
	Forwarded       = "Forwarded"
	XForwardedFor   = "X-Forwarded-For"
	XForwardedHost  = "X-Forwarded-Host"
	XForwardedProto = "X-Forwarded-Proto"

	// Distributed tracing and correlation headers.
	XRequestID     = "X-Request-Id"
	XCorrelationID = "X-Correlation-Id"
)

// standardHeaders contains canonical representations of standard HTTP and W3C headers.
var standardHeaders = map[string]struct{}{
	"Accept":                 {},
	"Accept-Charset":         {},
	"Accept-Encoding":        {},
	"Accept-Language":        {},
	"Accept-Ranges":          {},
	"Age":                    {},
	"Allow":                  {},
	"Authorization":          {},
	"Cache-Control":          {},
	"Connection":             {},
	"Content-Disposition":    {},
	"Content-Encoding":       {},
	"Content-Language":       {},
	"Content-Length":         {},
	"Content-Location":       {},
	"Content-Range":          {},
	"Content-Type":           {},
	"Cookie":                 {},
	"Date":                   {},
	"ETag":                   {},
	"Expect":                 {},
	"Expires":                {},
	"Forwarded":              {},
	"From":                   {},
	"Host":                   {},
	"If-Match":               {},
	"If-Modified-Since":      {},
	"If-None-Match":          {},
	"If-Range":               {},
	"If-Unmodified-Since":    {},
	"Last-Modified":          {},
	"Location":               {},
	"Max-Forwards":           {},
	"Origin":                 {},
	"Pragma":                 {},
	"Proxy-Authenticate":     {},
	"Proxy-Authorization":    {},
	"Range":                  {},
	"Referer":                {},
	"Retry-After":            {},
	"Sec-WebSocket-Accept":   {},
	"Sec-WebSocket-Key":      {},
	"Sec-WebSocket-Protocol": {},
	"Sec-WebSocket-Version":  {},
	"Server":                 {},
	"Set-Cookie":             {},
	"TE":                     {},
	"Traceparent":            {},
	"Tracestate":             {},
	"Trailer":                {},
	"Transfer-Encoding":      {},
	"Upgrade":                {},
	"User-Agent":             {},
	"Vary":                   {},
	"Via":                    {},
	"Warning":                {},
	"WWW-Authenticate":       {},
}

// IsStandard reports whether header is a recognized standard HTTP/W3C header.
func IsStandard(header string) bool {
	canonical := CanonicalKey(strings.TrimSpace(header))
	_, ok := standardHeaders[canonical]
	return ok
}

// IsCustom reports whether header is a custom or experimental header,
// characterized by the "X-" or "Zqk-" prefix (RFC 6648).
func IsCustom(header string) bool {
	canonical := CanonicalKey(strings.TrimSpace(header))
	return strings.HasPrefix(canonical, "X-") || strings.HasPrefix(canonical, "Zqk-")
}
