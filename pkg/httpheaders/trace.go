// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package httpheaders

import (
	"net/http"
	"strings"
)

// ExtractTraceContext extracts W3C distributed trace context headers (traceparent and tracestate)
// from the provided http.Header. Returns empty strings if headers are absent or h is nil.
func ExtractTraceContext(h http.Header) (traceparent, tracestate string) {
	if h == nil {
		return "", ""
	}
	return strings.TrimSpace(GetCanonical(h, Traceparent)), strings.TrimSpace(GetCanonical(h, Tracestate))
}

// InjectTraceContext injects W3C distributed trace context headers into the provided http.Header.
// If traceparent or tracestate are non-empty, they are set using canonical header keys.
// If h is nil, InjectTraceContext does nothing.
func InjectTraceContext(h http.Header, traceparent, tracestate string) {
	if h == nil {
		return
	}
	if tp := strings.TrimSpace(traceparent); tp != "" {
		SetCanonical(h, Traceparent, tp)
	}
	if ts := strings.TrimSpace(tracestate); ts != "" {
		SetCanonical(h, Tracestate, ts)
	}
}
