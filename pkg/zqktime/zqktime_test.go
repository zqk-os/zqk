package zqktime

import (
	"strings"
	"testing"
	"time"
)

func TestFormatRFC3339UTC_usesZForUTCInstant(t *testing.T) {
	t.Parallel()
	// Fixed instant: noon UTC
	tt := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	got := FormatRFC3339UTC(tt)
	if got != "2026-04-04T12:00:00Z" {
		t.Fatalf("got %q", got)
	}
	// Same instant in New York should still format as UTC Z
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	local := tt.In(loc)
	got2 := FormatRFC3339UTC(local)
	if got2 != "2026-04-04T12:00:00Z" {
		t.Fatalf("local input: got %q", got2)
	}
}

func TestNowRFC3339UTC_parsesAsRFC3339(t *testing.T) {
	t.Parallel()
	s := NowRFC3339UTC()
	_, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.HasSuffix(s, "Z") && !strings.Contains(s, "+00:00") {
		t.Fatalf("expected UTC suffix in %q", s)
	}
}

func TestFormatRFC3339UTCPtr(t *testing.T) {
	t.Parallel()
	if s := FormatRFC3339UTCPtr(nil); s != "" {
		t.Fatalf("nil: got %q", s)
	}
	tt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if g := FormatRFC3339UTCPtr(&tt); g != "2026-01-02T03:04:05Z" {
		t.Fatalf("got %q", g)
	}
}

func TestFormatLayoutUTC(t *testing.T) {
	t.Parallel()
	tt := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	if g := FormatLayoutUTC(tt, LayoutObjectDateTimeZ); g != "2026-01-02T15:04:05Z" {
		t.Fatalf("got %q", g)
	}
}
