package zqktime

import (
	"testing"
	"time"
)

func TestLayoutUTCStamps_doNotDependOnLocalZone(t *testing.T) {
	t.Parallel()
	tt := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	local := tt.In(loc)
	cases := []struct {
		layout string
		want   string
	}{
		{LayoutDateTimeSpace, "2026-01-02 15:04:05"},
		{LayoutDateTimeMinute, "2026-01-02 15:04"},
		{LayoutDate, "2026-01-02"},
		{LayoutMonth, "2026-01"},
		{LayoutLogRotateStamp, "20260102-150405"},
		{LayoutCompactStampZ, "20260102T150405Z"},
		{LayoutCompactHour, "20260102T1504"},
		{LayoutCompactDate, "20260102"},
	}
	for _, tc := range cases {
		if g := FormatLayoutUTC(local, tc.layout); g != tc.want {
			t.Fatalf("%s: got %q want %q", tc.layout, g, tc.want)
		}
	}
}
