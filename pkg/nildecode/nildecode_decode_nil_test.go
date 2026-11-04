// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package nildecode

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type sampleStruct struct {
	Name  string
	Count int
}

type nonComparableStruct struct {
	Tags []string
}

func TestDecodeNonNilPayload_Comprehensive(t *testing.T) {
	// Untyped nil
	if _, ok := DecodeNonNilPayload[string](nil); ok {
		t.Fatal("expected untyped nil to fail DecodeNonNilPayload")
	}

	// Typed nil pointer
	var nilPtr *int
	if _, ok := DecodeNonNilPayload[*int](nilPtr); ok {
		t.Fatal("expected typed nil pointer to fail DecodeNonNilPayload")
	}

	// Valid pointer
	val := 42
	gotPtr, ok := DecodeNonNilPayload[*int](&val)
	if !ok || gotPtr != &val || *gotPtr != 42 {
		t.Fatalf("expected valid pointer, got %v, ok=%v", gotPtr, ok)
	}

	// Non-pointer primitives: zero values
	if _, ok := DecodeNonNilPayload[int](0); ok {
		t.Fatal("expected zero int to fail DecodeNonNilPayload")
	}
	if _, ok := DecodeNonNilPayload[string](""); ok {
		t.Fatal("expected empty string to fail DecodeNonNilPayload")
	}
	if _, ok := DecodeNonNilPayload[bool](false); ok {
		t.Fatal("expected false bool to fail DecodeNonNilPayload")
	}

	// Non-pointer primitives: non-zero values
	if got, ok := DecodeNonNilPayload[int](100); !ok || got != 100 {
		t.Fatalf("expected int 100, got %d, ok=%v", got, ok)
	}
	if got, ok := DecodeNonNilPayload[string]("valid"); !ok || got != "valid" {
		t.Fatalf("expected 'valid', got %s, ok=%v", got, ok)
	}
	if got, ok := DecodeNonNilPayload[bool](true); !ok || !got {
		t.Fatalf("expected true, got %v, ok=%v", got, ok)
	}

	// Structs: zero vs non-zero
	var zeroStruct sampleStruct
	if _, ok := DecodeNonNilPayload[sampleStruct](zeroStruct); ok {
		t.Fatal("expected zero struct to fail DecodeNonNilPayload")
	}
	populated := sampleStruct{Name: "omega", Count: 7}
	if got, ok := DecodeNonNilPayload[sampleStruct](populated); !ok || got != populated {
		t.Fatalf("expected populated struct, got %+v, ok=%v", got, ok)
	}

	// Type mismatch
	if _, ok := DecodeNonNilPayload[int]("string-payload"); ok {
		t.Fatal("expected type mismatch to fail DecodeNonNilPayload")
	}
}

func TestDecodeOr(t *testing.T) {
	// Valid value
	if got := DecodeOr[string]("primary", "fallback"); got != "primary" {
		t.Fatalf("expected primary, got %s", got)
	}

	// Nil value falling back
	if got := DecodeOr[string](nil, "fallback"); got != "fallback" {
		t.Fatalf("expected fallback for nil, got %s", got)
	}

	// Zero value falling back
	if got := DecodeOr[string]("", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback for empty string, got %s", got)
	}
	if got := DecodeOr[int](0, 999); got != 999 {
		t.Fatalf("expected fallback 999 for zero int, got %d", got)
	}

	// Type mismatch fallback
	if got := DecodeOr[string](12345, "fallback"); got != "fallback" {
		t.Fatalf("expected fallback for type mismatch, got %s", got)
	}

	// Pointer nil fallback
	var nilPtr *int
	val := 10
	if got := DecodeOr[*int](nilPtr, &val); got != &val {
		t.Fatalf("expected fallback pointer, got %v", got)
	}
}

func TestMustDecode(t *testing.T) {
	// Success path: primitive
	strVal, err := MustDecode[string]("active")
	if err != nil || strVal != "active" {
		t.Fatalf("expected active with no error, got %s, err=%v", strVal, err)
	}

	// Success path: pointer
	num := 88
	numPtr, err := MustDecode[*int](&num)
	if err != nil || numPtr != &num {
		t.Fatalf("expected non-nil pointer, got %v, err=%v", numPtr, err)
	}

	// Error path: nil payload
	_, errNil := MustDecode[string](nil)
	if errNil == nil {
		t.Fatal("expected error on nil payload")
	}
	if !errors.Is(errNil, ErrNilPayload) {
		t.Fatalf("expected ErrNilPayload, got %v", errNil)
	}
	if !strings.Contains(errNil.Error(), "payload is nil") {
		t.Fatalf("expected descriptive message, got %s", errNil.Error())
	}

	// Error path: type mismatch
	_, errMismatch := MustDecode[int]("not-an-int")
	if errMismatch == nil {
		t.Fatal("expected error on type mismatch")
	}
	if !errors.Is(errMismatch, ErrTypeMismatch) {
		t.Fatalf("expected ErrTypeMismatch, got %v", errMismatch)
	}
	if !strings.Contains(errMismatch.Error(), "expected int") {
		t.Fatalf("expected descriptive type mismatch message, got %s", errMismatch.Error())
	}

	// Error path: zero value primitive
	_, errZeroInt := MustDecode[int](0)
	if errZeroInt == nil {
		t.Fatal("expected error on zero int")
	}
	if !errors.Is(errZeroInt, ErrZeroValue) {
		t.Fatalf("expected ErrZeroValue, got %v", errZeroInt)
	}
	if !strings.Contains(errZeroInt.Error(), "zero value") {
		t.Fatalf("expected zero value message, got %s", errZeroInt.Error())
	}

	// Error path: zero value string
	_, errZeroStr := MustDecode[string]("")
	if errZeroStr == nil {
		t.Fatal("expected error on zero string")
	}
	if !errors.Is(errZeroStr, ErrZeroValue) {
		t.Fatalf("expected ErrZeroValue, got %v", errZeroStr)
	}

	// Error path: typed nil pointer zero value
	var typedNil *int
	_, errZeroPtr := MustDecode[*int](typedNil)
	if errZeroPtr == nil {
		t.Fatal("expected error on typed nil pointer")
	}
	if !errors.Is(errZeroPtr, ErrZeroValue) {
		t.Fatalf("expected ErrZeroValue for typed nil pointer, got %v", errZeroPtr)
	}
}

func TestDecodeSlice(t *testing.T) {
	// Populated slice
	populated := []string{"alpha", "beta"}
	got, ok := DecodeSlice[string](populated)
	if !ok || len(got) != 2 || got[0] != "alpha" {
		t.Fatalf("expected populated slice, got %v, ok=%v", got, ok)
	}

	// Non-nil empty slice (should succeed)
	empty := []string{}
	gotEmpty, ok := DecodeSlice[string](empty)
	if !ok || gotEmpty == nil || len(gotEmpty) != 0 {
		t.Fatalf("expected non-nil empty slice to succeed, got %v, ok=%v", gotEmpty, ok)
	}

	// Nil slice (should fail)
	var nilSlice []string
	if _, ok := DecodeSlice[string](nilSlice); ok {
		t.Fatal("expected nil slice to fail DecodeSlice")
	}

	// Untyped nil (should fail)
	if _, ok := DecodeSlice[string](nil); ok {
		t.Fatal("expected untyped nil to fail DecodeSlice")
	}

	// Type mismatch (wrong element type)
	if _, ok := DecodeSlice[int](populated); ok {
		t.Fatal("expected element type mismatch to fail DecodeSlice")
	}

	// Non-slice payload
	if _, ok := DecodeSlice[string]("just-a-string"); ok {
		t.Fatal("expected non-slice payload to fail DecodeSlice")
	}
}

func TestDecodeMap(t *testing.T) {
	// Populated map
	populated := map[string]int{"k1": 10, "k2": 20}
	got, ok := DecodeMap[string, int](populated)
	if !ok || len(got) != 2 || got["k1"] != 10 {
		t.Fatalf("expected populated map, got %v, ok=%v", got, ok)
	}

	// Non-nil empty map (should succeed)
	empty := map[string]int{}
	gotEmpty, ok := DecodeMap[string, int](empty)
	if !ok || gotEmpty == nil || len(gotEmpty) != 0 {
		t.Fatalf("expected non-nil empty map to succeed, got %v, ok=%v", gotEmpty, ok)
	}

	// Nil map (should fail)
	var nilMap map[string]int
	if _, ok := DecodeMap[string, int](nilMap); ok {
		t.Fatal("expected nil map to fail DecodeMap")
	}

	// Untyped nil (should fail)
	if _, ok := DecodeMap[string, int](nil); ok {
		t.Fatal("expected untyped nil to fail DecodeMap")
	}

	// Type mismatch (wrong value type)
	if _, ok := DecodeMap[string, string](populated); ok {
		t.Fatal("expected value type mismatch to fail DecodeMap")
	}

	// Non-map payload
	if _, ok := DecodeMap[string, int](42); ok {
		t.Fatal("expected non-map payload to fail DecodeMap")
	}
}

func TestDecodePointer(t *testing.T) {
	// Valid pointer to primitive
	val := 123
	got, ok := DecodePointer[int](&val)
	if !ok || got != &val || *got != 123 {
		t.Fatalf("expected valid pointer, got %v, ok=%v", got, ok)
	}

	// Valid pointer to non-comparable struct
	nonComp := &nonComparableStruct{Tags: []string{"a", "b"}}
	gotNonComp, ok := DecodePointer[nonComparableStruct](nonComp)
	if !ok || gotNonComp != nonComp || len(gotNonComp.Tags) != 2 {
		t.Fatalf("expected valid pointer to non-comparable struct, got %v, ok=%v", gotNonComp, ok)
	}

	// Nil pointer (should fail)
	var nilPtr *int
	if _, ok := DecodePointer[int](nilPtr); ok {
		t.Fatal("expected nil pointer to fail DecodePointer")
	}

	// Untyped nil (should fail)
	if _, ok := DecodePointer[int](nil); ok {
		t.Fatal("expected untyped nil to fail DecodePointer")
	}

	// Wrong pointer type
	str := "text"
	if _, ok := DecodePointer[int](&str); ok {
		t.Fatal("expected wrong pointer type to fail DecodePointer")
	}

	// Non-pointer value
	if _, ok := DecodePointer[int](val); ok {
		t.Fatal("expected non-pointer value to fail DecodePointer")
	}
}

func TestIsNilOrZero(t *testing.T) {
	// Untyped nil
	if !IsNilOrZero(nil) {
		t.Fatal("expected IsNilOrZero(nil) to be true")
	}

	// Typed nil pointer
	var nilIntPtr *int
	if !IsNilOrZero(nilIntPtr) {
		t.Fatal("expected IsNilOrZero(nilIntPtr) to be true")
	}

	// Typed nil struct pointer
	var nilStructPtr *sampleStruct
	if !IsNilOrZero(nilStructPtr) {
		t.Fatal("expected IsNilOrZero(nilStructPtr) to be true")
	}

	// Nil slice
	var nilSlice []string
	if !IsNilOrZero(nilSlice) {
		t.Fatal("expected IsNilOrZero(nilSlice) to be true")
	}

	// Nil map
	var nilMap map[string]int
	if !IsNilOrZero(nilMap) {
		t.Fatal("expected IsNilOrZero(nilMap) to be true")
	}

	// Nil channel
	var nilChan chan int
	if !IsNilOrZero(nilChan) {
		t.Fatal("expected IsNilOrZero(nilChan) to be true")
	}

	// Nil func
	var nilFunc func()
	if !IsNilOrZero(nilFunc) {
		t.Fatal("expected IsNilOrZero(nilFunc) to be true")
	}

	// Non-nil empty slice (should be false: allocated, not nil)
	emptySlice := []string{}
	if IsNilOrZero(emptySlice) {
		t.Fatal("expected IsNilOrZero(emptySlice) to be false")
	}

	// Non-nil populated slice
	populatedSlice := []string{"data"}
	if IsNilOrZero(populatedSlice) {
		t.Fatal("expected IsNilOrZero(populatedSlice) to be false")
	}

	// Non-nil empty map (should be false)
	emptyMap := map[string]int{}
	if IsNilOrZero(emptyMap) {
		t.Fatal("expected IsNilOrZero(emptyMap) to be false")
	}

	// Non-nil channel
	activeChan := make(chan int)
	if IsNilOrZero(activeChan) {
		t.Fatal("expected IsNilOrZero(activeChan) to be false")
	}

	// Non-nil func
	activeFunc := func() {}
	if IsNilOrZero(activeFunc) {
		t.Fatal("expected IsNilOrZero(activeFunc) to be false")
	}

	// Zero primitives
	if !IsNilOrZero(0) {
		t.Fatal("expected IsNilOrZero(0) to be true")
	}
	if !IsNilOrZero(int64(0)) {
		t.Fatal("expected IsNilOrZero(int64(0)) to be true")
	}
	if !IsNilOrZero(uint(0)) {
		t.Fatal("expected IsNilOrZero(uint(0)) to be true")
	}
	if !IsNilOrZero(0.0) {
		t.Fatal("expected IsNilOrZero(0.0) to be true")
	}
	if !IsNilOrZero("") {
		t.Fatal("expected IsNilOrZero(\"\") to be true")
	}
	if !IsNilOrZero(false) {
		t.Fatal("expected IsNilOrZero(false) to be true")
	}
	if !IsNilOrZero(sampleStruct{}) {
		t.Fatal("expected IsNilOrZero(emptyStruct) to be true")
	}

	// Non-zero values
	if IsNilOrZero(42) {
		t.Fatal("expected IsNilOrZero(42) to be false")
	}
	if IsNilOrZero("non-empty") {
		t.Fatal("expected IsNilOrZero(\"non-empty\") to be false")
	}
	if IsNilOrZero(true) {
		t.Fatal("expected IsNilOrZero(true) to be false")
	}
	if IsNilOrZero(sampleStruct{Name: "test"}) {
		t.Fatal("expected IsNilOrZero(populatedStruct) to be false")
	}

	// Allocated non-nil pointer pointing to zero value (pointer itself is non-nil/non-zero)
	zeroInt := 0
	if IsNilOrZero(&zeroInt) {
		t.Fatal("expected allocated pointer to zero int to be false (pointer itself is non-nil)")
	}
}

func TestConcurrency_Safety(t *testing.T) {
	const numGoroutines = 25
	const iterations = 50

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines*iterations)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		workerID := i
		builder := goroutinelabels.NewGoroutine("test_nildecode_concurrency", fmt.Sprintf("concurrency worker %d", workerID))
		builder.StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Concurrently test DecodeNonNilPayload
				val, ok := DecodeNonNilPayload[int](j + 1)
				if !ok || val != j+1 {
					errCh <- fmt.Errorf("DecodeNonNilPayload failed for %d in worker %d", j+1, workerID)
					return
				}

				// Concurrently test DecodeOr
				fallbackVal := DecodeOr[string](nil, "safe")
				if fallbackVal != "safe" {
					errCh <- fmt.Errorf("DecodeOr fallback failed in worker %d", workerID)
					return
				}

				// Concurrently test MustDecode
				mVal, mErr := MustDecode[string]("concurrent-str")
				if mErr != nil || mVal != "concurrent-str" {
					errCh <- fmt.Errorf("MustDecode failed in worker %d: %v", workerID, mErr)
					return
				}

				// Concurrently test DecodeSlice
				sl, sOk := DecodeSlice[int]([]int{workerID, j})
				if !sOk || len(sl) != 2 {
					errCh <- fmt.Errorf("DecodeSlice failed in worker %d", workerID)
					return
				}

				// Concurrently test DecodeMap
				mp, mOk := DecodeMap[string, int](map[string]int{"k": j})
				if !mOk || mp["k"] != j {
					errCh <- fmt.Errorf("DecodeMap failed in worker %d", workerID)
					return
				}

				// Concurrently test DecodePointer
				ptr := &workerID
				pVal, pOk := DecodePointer[int](ptr)
				if !pOk || *pVal != workerID {
					errCh <- fmt.Errorf("DecodePointer failed in worker %d", workerID)
					return
				}

				// Concurrently test IsNilOrZero
				if !IsNilOrZero(0) {
					errCh <- fmt.Errorf("IsNilOrZero(0) returned false in worker %d", workerID)
					return
				}
				if IsNilOrZero("active") {
					errCh <- fmt.Errorf("IsNilOrZero('active') returned true in worker %d", workerID)
					return
				}
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}
}
