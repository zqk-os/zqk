package system

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func newAggregateAuditFlagsCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("window", "24h", "Time window for aggregation")
	cmd.Flags().String("start", "", "Start time for aggregation window")
	cmd.Flags().String("end", "", "End time for aggregation window")
	cmd.Flags().Bool("archive", false, "Archive processed events")
	cmd.Flags().Bool("delete", false, "Delete processed events")
	cmd.Flags().String("cleanup-metric", "", "Cleanup metric id")
	return cmd
}

func TestParseDuration_AggregateAudit(t *testing.T) {
	tests := []struct {
		in     string
		wantH  time.Duration
		wantOk bool
	}{
		{in: "24h", wantH: 24 * time.Hour, wantOk: true},
		{in: "7d", wantH: 7 * 24 * time.Hour, wantOk: true},
		{in: "1w", wantH: 7 * 24 * time.Hour, wantOk: true},
		{in: "2m", wantH: 2 * 30 * 24 * time.Hour, wantOk: true},
		{in: "bad", wantOk: false},
		{in: "x", wantOk: false},
	}

	for _, tt := range tests {
		got, err := parseDuration(tt.in)
		if tt.wantOk && err != nil {
			t.Fatalf("parseDuration(%q) unexpected err: %v", tt.in, err)
		}
		if !tt.wantOk {
			if err == nil {
				t.Fatalf("parseDuration(%q) expected err, got nil", tt.in)
			}
			continue
		}
		if got != tt.wantH {
			t.Fatalf("parseDuration(%q)=%v want %v", tt.in, got, tt.wantH)
		}
	}
}

func TestParseTimeWindowWithNow_AggregateAudit(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("explicit start/end", func(t *testing.T) {
		cmd := newAggregateAuditFlagsCmd()
		_ = cmd.Flags().Set("start", "2025-12-01T00:00:00Z")
		_ = cmd.Flags().Set("end", "2025-12-02T00:00:00Z")

		start, end, err := parseTimeWindowWithNow(cmd, now)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !start.Equal(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("unexpected start: %v", start)
		}
		if !end.Equal(time.Date(2025, 12, 2, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("unexpected end: %v", end)
		}
	})

	t.Run("explicit start only uses nowUTC as end", func(t *testing.T) {
		cmd := newAggregateAuditFlagsCmd()
		_ = cmd.Flags().Set("start", "2025-12-01T00:00:00Z")
		_ = cmd.Flags().Set("end", "")

		start, end, err := parseTimeWindowWithNow(cmd, now)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !start.Equal(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("unexpected start: %v", start)
		}
		if !end.Equal(now) {
			t.Fatalf("unexpected end: %v want %v", end, now)
		}
	})

	t.Run("window duration uses nowUTC as end", func(t *testing.T) {
		cmd := newAggregateAuditFlagsCmd()
		_ = cmd.Flags().Set("window", "24h")
		_ = cmd.Flags().Set("start", "")
		_ = cmd.Flags().Set("end", "")

		start, end, err := parseTimeWindowWithNow(cmd, now)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !end.Equal(now) {
			t.Fatalf("unexpected end: %v want %v", end, now)
		}
		if !start.Equal(now.Add(-24 * time.Hour)) {
			t.Fatalf("unexpected start: %v want %v", start, now.Add(-24*time.Hour))
		}
	})

	t.Run("invalid window errors", func(t *testing.T) {
		cmd := newAggregateAuditFlagsCmd()
		_ = cmd.Flags().Set("window", "bad")
		_ = cmd.Flags().Set("start", "")
		_ = cmd.Flags().Set("end", "")

		_, _, err := parseTimeWindowWithNow(cmd, now)
		if err == nil {
			t.Fatalf("expected err, got nil")
		}
	})
}

func TestParseAggregateAuditCleanupDecisionFromFlags(t *testing.T) {
	t.Run("cleanup-metric requires archive or delete", func(t *testing.T) {
		cmd := newAggregateAuditFlagsCmd()
		_ = cmd.Flags().Set("cleanup-metric", "AAM-001")
		_, err := parseAggregateAuditCleanupDecisionFromFlags(cmd)
		if err == nil {
			t.Fatalf("expected err, got nil")
		}
	})

	t.Run("cleanup-metric delete takes precedence over archive", func(t *testing.T) {
		cmd := newAggregateAuditFlagsCmd()
		_ = cmd.Flags().Set("cleanup-metric", "AAM-001")
		_ = cmd.Flags().Set("archive", "true")
		_ = cmd.Flags().Set("delete", "true")

		dec, err := parseAggregateAuditCleanupDecisionFromFlags(cmd)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if dec.Mode != aggregateAuditCleanupMetricDelete {
			t.Fatalf("unexpected mode: %v", dec.Mode)
		}
		if dec.MetricID != "AAM-001" {
			t.Fatalf("unexpected metricID: %v", dec.MetricID)
		}
	})

	t.Run("cleanup-metric archive", func(t *testing.T) {
		cmd := newAggregateAuditFlagsCmd()
		_ = cmd.Flags().Set("cleanup-metric", "AAM-001")
		_ = cmd.Flags().Set("archive", "true")
		_ = cmd.Flags().Set("delete", "false")

		dec, err := parseAggregateAuditCleanupDecisionFromFlags(cmd)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if dec.Mode != aggregateAuditCleanupMetricArchive {
			t.Fatalf("unexpected mode: %v", dec.Mode)
		}
	})

	t.Run("delete without cleanup-metric", func(t *testing.T) {
		cmd := newAggregateAuditFlagsCmd()
		_ = cmd.Flags().Set("delete", "true")

		dec, err := parseAggregateAuditCleanupDecisionFromFlags(cmd)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if dec.Mode != aggregateAuditCleanupDelete {
			t.Fatalf("unexpected mode: %v", dec.Mode)
		}
	})

	t.Run("archive without cleanup-metric", func(t *testing.T) {
		cmd := newAggregateAuditFlagsCmd()
		_ = cmd.Flags().Set("archive", "true")

		dec, err := parseAggregateAuditCleanupDecisionFromFlags(cmd)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if dec.Mode != aggregateAuditCleanupArchive {
			t.Fatalf("unexpected mode: %v", dec.Mode)
		}
	})

	t.Run("no cleanup flags", func(t *testing.T) {
		cmd := newAggregateAuditFlagsCmd()
		dec, err := parseAggregateAuditCleanupDecisionFromFlags(cmd)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if dec.Mode != aggregateAuditCleanupNone {
			t.Fatalf("unexpected mode: %v", dec.Mode)
		}
	})
}
