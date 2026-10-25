// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package nildecode

import "reflect"

// DecodePointer asserts payload is a non-nil pointer of type *T.
// Unlike DecodeNonNilPayload, T is not required to be comparable, enabling safe
// extraction of pointers to structs containing slices, maps, or functions.
func DecodePointer[T any](payload any) (*T, bool) {
	if payload == nil {
		return nil, false
	}
	p, ok := payload.(*T)
	if !ok || p == nil {
		return nil, false
	}
	return p, true
}

// DecodeSlice asserts payload is a non-nil slice of type []T.
// It distinguishes nil slices (which return false) from empty slices (which return true).
func DecodeSlice[T any](payload any) ([]T, bool) {
	if payload == nil {
		return nil, false
	}
	s, ok := payload.([]T)
	if !ok || s == nil {
		return nil, false
	}
	return s, true
}

// DecodeMap asserts payload is a non-nil map of type map[K]V.
// It distinguishes nil maps (which return false) from empty maps (which return true).
func DecodeMap[K comparable, V any](payload any) (map[K]V, bool) {
	if payload == nil {
		return nil, false
	}
	m, ok := payload.(map[K]V)
	if !ok || m == nil {
		return nil, false
	}
	return m, true
}

// IsNilOrZero reports whether v is nil, an interface wrapping a nil pointer/reference,
// or the zero value for its concrete type. It safely inspects pointers, interfaces, channels,
// maps, slices, funcs, structs, and primitive types without panicking.
func IsNilOrZero(v any) bool {
	if v == nil {
		return true
	}
	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		if val.IsNil() {
			return true
		}
	}
	return val.IsZero()
}
