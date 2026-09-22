// BLI-STARTER-COMMUNITY-026 / PRI-STARTER-COMMUNITY-026 coverage elevation
package zqktime

import (
	"strings"
	"testing"
	"time"
)

func TestFormatRFC3339NanoUTC(t *testing.T) {
	t.Parallel()
	tt := time.Date(2026, 9, 22, 12, 30, 45, 123456789, time.UTC)
	got := FormatRFC3339NanoUTC(tt)
	if !strings.HasPrefix(got, "2026-09-22T12:30:45") || !strings.HasSuffix(got, "Z") {
		t.Fatalf("got %q", got)
	}
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip(err)
	}
	got2 := FormatRFC3339NanoUTC(tt.In(loc))
	if got2 != got {
		t.Fatalf("zone leak: %q vs %q", got, got2)
	}
}

func TestNowRFC3339NanoUTC_parses(t *testing.T) {
	t.Parallel()
	s := NowRFC3339NanoUTC()
	if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

func TestNowLayoutUTC_Layouts(t *testing.T) {
	t.Parallel()
	for _, layout := range []string{
		LayoutDateTimeSpace,
		LayoutDateTimeMinute,
		LayoutDateTimeMillis,
		LayoutObjectDateTimeZ,
		LayoutDate,
		LayoutMonth,
		LayoutLogRotateStamp,
		LayoutCompactStampZ,
		LayoutCompactHour,
		LayoutCompactDate,
	} {
		got := NowLayoutUTC(layout)
		if got == "" {
			t.Fatalf("empty for %q", layout)
		}
		if _, err := time.Parse(layout, got); err != nil {
			t.Fatalf("parse %q %q: %v", layout, got, err)
		}
	}
}
