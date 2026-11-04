// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package zqktime

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestAllLayoutConstants_FormatValidation(t *testing.T) {
	t.Parallel()

	fixedInstant := time.Date(2026, 4, 4, 12, 30, 45, 123000000, time.UTC)
	testCases := []struct {
		name     string
		layout   string
		expected string
	}{
		{
			name:     "LayoutDateTimeSpace",
			layout:   LayoutDateTimeSpace,
			expected: "2026-04-04 12:30:45",
		},
		{
			name:     "LayoutDateTimeMinute",
			layout:   LayoutDateTimeMinute,
			expected: "2026-04-04 12:30",
		},
		{
			name:     "LayoutDateTimeMillis",
			layout:   LayoutDateTimeMillis,
			expected: "2026-04-04 12:30:45.123",
		},
		{
			name:     "LayoutObjectDateTimeZ",
			layout:   LayoutObjectDateTimeZ,
			expected: "2026-04-04T12:30:45Z",
		},
		{
			name:     "LayoutDate",
			layout:   LayoutDate,
			expected: "2026-04-04",
		},
		{
			name:     "LayoutMonth",
			layout:   LayoutMonth,
			expected: "2026-04",
		},
		{
			name:     "LayoutLogRotateStamp",
			layout:   LayoutLogRotateStamp,
			expected: "20260404-123045",
		},
		{
			name:     "LayoutCompactStampZ",
			layout:   LayoutCompactStampZ,
			expected: "20260404T123045Z",
		},
		{
			name:     "LayoutCompactHour",
			layout:   LayoutCompactHour,
			expected: "20260404T1230",
		},
		{
			name:     "LayoutCompactDate",
			layout:   LayoutCompactDate,
			expected: "20260404",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FormatLayoutUTC(fixedInstant, tc.layout)
			if got != tc.expected {
				t.Fatalf("layout %s: got %q, want %q", tc.name, got, tc.expected)
			}
		})
	}
}

func TestFormatRFC3339UTC_ArbitraryLocationsAndEdgeDates(t *testing.T) {
	t.Parallel()

	nyLoc := time.FixedZone("EST", -5*3600)
	tokyoLoc := time.FixedZone("JST", 9*3600)
	istLoc := time.FixedZone("IST", 5*3600+1800) // +05:30

	testCases := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "EpochZero",
			input:    time.Unix(0, 0).UTC(),
			expected: "1970-01-01T00:00:00Z",
		},
		{
			name:     "EpochZero_InNewYorkZone",
			input:    time.Unix(0, 0).In(nyLoc),
			expected: "1970-01-01T00:00:00Z",
		},
		{
			name:     "LeapYear2024",
			input:    time.Date(2024, 2, 29, 23, 59, 59, 0, tokyoLoc),
			expected: "2024-02-29T14:59:59Z",
		},
		{
			name:     "IndiaOffsetQuarterHour",
			input:    time.Date(2026, 7, 15, 6, 0, 0, 0, istLoc),
			expected: "2026-07-15T00:30:00Z",
		},
		{
			name:     "DistantPastYear1899",
			input:    time.Date(1899, 12, 31, 23, 59, 59, 0, time.UTC),
			expected: "1899-12-31T23:59:59Z",
		},
		{
			name:     "DistantFutureYear2999",
			input:    time.Date(2999, 12, 31, 23, 59, 59, 0, time.UTC),
			expected: "2999-12-31T23:59:59Z",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FormatRFC3339UTC(tc.input)
			if got != tc.expected {
				t.Fatalf("FormatRFC3339UTC(%v): got %q, want %q", tc.input, got, tc.expected)
			}
			parsed, err := time.Parse(time.RFC3339, got)
			if err != nil {
				t.Fatalf("time.Parse failed on %q: %v", got, err)
			}
			if !parsed.Equal(tc.input) {
				t.Fatalf("parsed time %v not equal to input %v", parsed, tc.input)
			}
		})
	}
}

func TestNowRFC3339UTC_SanityAndPrecision(t *testing.T) {
	t.Parallel()

	before := time.Now().UTC().Add(-2 * time.Second)
	stamp := NowRFC3339UTC()
	after := time.Now().UTC().Add(2 * time.Second)

	parsed, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatalf("NowRFC3339UTC produced unparseable stamp %q: %v", stamp, err)
	}
	if parsed.Before(before) || parsed.After(after) {
		t.Fatalf("timestamp %v out of expected window [%v, %v]", parsed, before, after)
	}
	if !strings.HasSuffix(stamp, "Z") && !strings.Contains(stamp, "+00:00") {
		t.Fatalf("expected UTC suffix in stamp %q", stamp)
	}
}

func TestFormatRFC3339UTCPtr_NilAndNonNilCases(t *testing.T) {
	t.Parallel()

	if nilRes := FormatRFC3339UTCPtr(nil); nilRes != "" {
		t.Fatalf("expected empty string for nil pointer, got %q", nilRes)
	}

	sydneyLoc := time.FixedZone("AEST", 10*3600)
	localInstant := time.Date(2026, 10, 24, 20, 0, 0, 0, sydneyLoc)
	res := FormatRFC3339UTCPtr(&localInstant)
	want := "2026-10-24T10:00:00Z"
	if res != want {
		t.Fatalf("FormatRFC3339UTCPtr(&localInstant): got %q, want %q", res, want)
	}
}

func TestFormatRFC3339NanoUTC_SubmillisecondFidelity(t *testing.T) {
	t.Parallel()

	testInstants := []struct {
		name      string
		t         time.Time
		wantNanos int
	}{
		{
			name:      "SingleNanosecond",
			t:         time.Date(2026, 3, 1, 10, 11, 12, 1, time.UTC),
			wantNanos: 1,
		},
		{
			name:      "MaxNanoseconds",
			t:         time.Date(2026, 3, 1, 10, 11, 12, 999999999, time.UTC),
			wantNanos: 999999999,
		},
		{
			name:      "MicrosecondFidelity",
			t:         time.Date(2026, 3, 1, 10, 11, 12, 123456000, time.UTC),
			wantNanos: 123456000,
		},
		{
			name:      "NonUTCZoneWithNanos",
			t:         time.Date(2026, 3, 1, 15, 41, 12, 555666777, time.FixedZone("IST", 5*3600+1800)),
			wantNanos: 555666777,
		},
	}

	for _, tc := range testInstants {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			formatted := FormatRFC3339NanoUTC(tc.t)
			parsed, err := time.Parse(time.RFC3339Nano, formatted)
			if err != nil {
				t.Fatalf("failed parsing nano stamp %q: %v", formatted, err)
			}
			if !parsed.Equal(tc.t) {
				t.Fatalf("parsed %v != input %v", parsed, tc.t)
			}
			if parsed.Nanosecond() != tc.wantNanos {
				t.Fatalf("nanosecond mismatch: got %d, want %d", parsed.Nanosecond(), tc.wantNanos)
			}
			if !strings.HasSuffix(formatted, "Z") {
				t.Fatalf("nano stamp %q missing Z suffix", formatted)
			}
		})
	}
}

func TestNowRFC3339NanoUTC_SanityAndPrecision(t *testing.T) {
	t.Parallel()

	before := time.Now().UTC().Add(-2 * time.Second)
	stamp := NowRFC3339NanoUTC()
	after := time.Now().UTC().Add(2 * time.Second)

	parsed, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		t.Fatalf("NowRFC3339NanoUTC produced unparseable stamp %q: %v", stamp, err)
	}
	if parsed.Before(before) || parsed.After(after) {
		t.Fatalf("nano timestamp %v out of bounds [%v, %v]", parsed, before, after)
	}
}

func TestFormatLayoutUTC_And_NowLayoutUTC_Custom(t *testing.T) {
	t.Parallel()

	customLayout := "Monday, 02-Jan-2006 15:04:05.000 MST"
	instant := time.Date(2026, 5, 10, 8, 30, 0, 500000000, time.FixedZone("PST", -8*3600))

	got := FormatLayoutUTC(instant, customLayout)
	want := "Sunday, 10-May-2026 16:30:00.500 UTC"
	if got != want {
		t.Fatalf("FormatLayoutUTC custom: got %q, want %q", got, want)
	}

	nowCustom := NowLayoutUTC(customLayout)
	if !strings.HasSuffix(nowCustom, "UTC") {
		t.Fatalf("NowLayoutUTC custom does not contain UTC: %q", nowCustom)
	}
}

func TestWindowStrings_BoundariesAndFormatting(t *testing.T) {
	t.Parallel()

	laZone := time.FixedZone("PDT", -7*3600)
	parisZone := time.FixedZone("CEST", 2*3600)

	start := time.Date(2026, 6, 1, 9, 0, 0, 0, laZone)
	end := time.Date(2026, 6, 1, 18, 0, 0, 0, parisZone)

	before := time.Now().UTC().Add(-2 * time.Second)
	nowInst, startStr, endStr := WindowStrings(start, end)
	after := time.Now().UTC().Add(2 * time.Second)

	if nowInst.Before(before) || nowInst.After(after) {
		t.Fatalf("WindowStrings now %v out of window [%v, %v]", nowInst, before, after)
	}

	expectedStart := "2026-06-01T16:00:00Z"
	if startStr != expectedStart {
		t.Fatalf("startStr mismatch: got %q, want %q", startStr, expectedStart)
	}

	expectedEnd := "2026-06-01T16:00:00Z"
	if endStr != expectedEnd {
		t.Fatalf("endStr mismatch: got %q, want %q", endStr, expectedEnd)
	}

	if nowInst.Location() != time.UTC {
		t.Fatalf("WindowStrings nowInst location is not UTC: %v", nowInst.Location())
	}
}

func TestDeterministicTimeSimulation_MonotonicityAndFrozenReplay(t *testing.T) {
	t.Parallel()

	baseInstant := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	frozenStamp1 := FormatRFC3339UTC(baseInstant)
	frozenStamp2 := FormatRFC3339UTC(baseInstant)
	if frozenStamp1 != frozenStamp2 {
		t.Fatalf("deterministic replay failed: %q != %q", frozenStamp1, frozenStamp2)
	}

	tickStep := 500 * time.Millisecond
	prev := baseInstant
	for i := 1; i <= 20; i++ {
		current := baseInstant.Add(time.Duration(i) * tickStep)
		curFormatted := FormatRFC3339NanoUTC(current)
		parsed, err := time.Parse(time.RFC3339Nano, curFormatted)
		if err != nil {
			t.Fatalf("tick %d unparseable: %v", i, err)
		}
		if !parsed.After(prev) {
			t.Fatalf("monotonicity violation at step %d: current %v not after prev %v", i, parsed, prev)
		}
		prev = parsed
	}
}

func TestConcurrency_FormatOperationsAcrossGoroutines(t *testing.T) {
	t.Parallel()

	const workerCount = 10
	const iterations = 50

	var wg sync.WaitGroup
	errCh := make(chan error, workerCount*iterations)

	for w := 0; w < workerCount; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("zqktime_concurrency_worker", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				runConcurrentFormatWorker(workerID, iterations, errCh)
			})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent format error: %v", err)
		}
	}
}

func runConcurrentFormatWorker(workerID, iterations int, errCh chan<- error) {
	fixedInstant := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < iterations; i++ {
		tVal := fixedInstant.Add(time.Duration(workerID*1000+i) * time.Millisecond)

		rfc := FormatRFC3339UTC(tVal)
		if !strings.HasSuffix(rfc, "Z") {
			errCh <- fmt.Errorf("worker %d: FormatRFC3339UTC missing Z: %q", workerID, rfc)
			return
		}

		nano := FormatRFC3339NanoUTC(tVal)
		if !strings.HasSuffix(nano, "Z") {
			errCh <- fmt.Errorf("worker %d: FormatRFC3339NanoUTC missing Z: %q", workerID, nano)
			return
		}

		_, sStr, eStr := WindowStrings(tVal, tVal.Add(time.Hour))
		if sStr == "" || eStr == "" {
			errCh <- fmt.Errorf("worker %d: WindowStrings returned empty", workerID)
			return
		}

		ptrStr := FormatRFC3339UTCPtr(&tVal)
		if ptrStr != rfc {
			errCh <- fmt.Errorf("worker %d: ptr mismatch %q vs %q", workerID, ptrStr, rfc)
			return
		}
	}
}
