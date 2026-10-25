// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

// Package nildecode provides generic helpers for type assertions with non-zero checks.
// Kept separate from pkg/pipeline so low-level packages (e.g. pkg/objects) can use them without
// importing pipeline (the pipeline package depends on pkg/objects in other files).
package nildecode

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by MustDecode.
var (
	ErrNilPayload   = errors.New("nildecode: payload is nil")
	ErrTypeMismatch = errors.New("nildecode: payload type mismatch")
	ErrZeroValue    = errors.New("nildecode: payload is zero value")
)

// DecodeNonNilPayload asserts payload has dynamic type P and is not P's zero value. For pointer
// payloads P (e.g. *FooPayload), the zero value is nil. P must be [comparable]; function and slice
// types cannot be used (use DecodeSlice or a manual assert for those).
func DecodeNonNilPayload[P comparable](payload any) (P, bool) {
	var z P
	p, ok := payload.(P)
	if !ok {
		return z, false
	}
	if p == z {
		return z, false
	}
	return p, true
}

// DecodeOr asserts payload has dynamic type P and is not P's zero value, returning fallback if
// invalid, nil, or zero.
func DecodeOr[P comparable](payload any, fallback P) P {
	if p, ok := DecodeNonNilPayload[P](payload); ok {
		return p
	}
	return fallback
}

// MustDecode asserts payload has dynamic type P and is not P's zero value.
// It returns an actionable error if payload is nil, wrong type, or zero value.
func MustDecode[P comparable](payload any) (P, error) {
	var z P
	if payload == nil {
		return z, ErrNilPayload
	}
	p, ok := payload.(P)
	if !ok {
		return z, fmt.Errorf("%w: expected %T, got %T", ErrTypeMismatch, z, payload)
	}
	if p == z {
		return z, fmt.Errorf("%w: %T has zero value", ErrZeroValue, z)
	}
	return p, nil
}
