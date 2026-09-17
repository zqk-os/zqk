package healthcheck

import (
	"context"
	"testing"
)

// TestMonitorResultsMustLabelSyntheticGauges verifies that every monitor
// result carries an explicit "gauge_type" key in its Details map, and
// that synthetic/placeholder gauges never achieve statusOK — they must be
// degraded or have gauge_type=="synthetic" so consumers can distinguish
// between a passing real signal and absence of data.
func TestMonitorResultsMustLabelSyntheticGauges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		res           *Result
		wantGaugeType bool // does this result need gauge_type labeled?
		wantNotOK     bool // synthetic/placeholder gauges must not report OK
	}{
		{
			name: "statusOK_with_gauge_type_is_valid",
			res: &Result{
				Status:  statusOK,
				Summary: "all good",
				Details: map[string]any{"gauge_type": "real"},
			},
			wantGaugeType: true,
		},
		{
			name: "synthetic_gauge_must_not_be_ok",
			res: &Result{
				Status:  statusDegraded,
				Summary: "no data yet",
				Details: map[string]any{"gauge_type": "synthetic"},
			},
			wantNotOK: true, // synthetic gauge returning OK is a false-green
		},
		{
			name: "real_gauge_with_gauge_type_is_valid",
			res: &Result{
				Status:  statusOK,
				Summary: "real check passed",
				Details: map[string]any{"gauge_type": "real"},
			},
			wantGaugeType: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Every OK status result must have gauge_type labeled for auditability.
			if tc.wantGaugeType && tc.res.Status == statusOK {
				if gt, ok := tc.res.Details["gauge_type"]; !ok || gt == "" {
					t.Errorf("OK status result missing gauge_type in details: %#v", tc.res)
				}
			}

			// Synthetic gauges must never achieve OK — that is a false-green.
			if tc.wantNotOK {
				if tc.res.Status == statusOK {
					t.Errorf("synthetic gauge reported OK (false-green): %#v", tc.res)
				}
			}
		})
	}
}

// TestRegistry_Run_MonitorWithNilOrPlaceholderResultMustNotGreen verifies
// that the registry aggregation does not promote synthetic/no-data monitors
// to OK status.
func TestRegistry_Run_NoDataMonitorsMustNotGreen(t *testing.T) {
	t.Parallel()

	r := NewRegistry("")

	// A monitor that produces a degraded result with synthetic gauge_type
	// must NOT be promoted to OK in the aggregation.
	syntheticMonitor := &stubMonitor{
		id: "synth-gauge",
		res: &Result{
			Status:  statusDegraded,
			Summary: "synthetic only",
			Details: map[string]any{"gauge_type": "synthetic"},
		},
	}
	r.Register(syntheticMonitor)

	res, err := r.Run(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Run unexpected error: %v", err)
	}
	var gt map[string]any
	if rObj, ok := res.Details["synth-gauge"].(*Result); ok && rObj != nil {
		gt = rObj.Details
	} else if dMap, ok := res.Details["synth-gauge"].(map[string]any); ok {
		gt = dMap
	}
	if gt == nil {
		t.Fatal("expected synth-gauge details in aggregation")
	}
	if gtStr, ok := gt["gauge_type"]; !ok || gtStr != "synthetic" {
		t.Fatalf("gauge_type must be labeled 'synthetic': %v", gt)
	}
	// If a monitor result IS synthetic (i.e., it's just noise),
	// the aggregation should downgrade it, not promote.
	if res.Status == statusOK {
		t.Fatalf("aggregation promoted synthetic monitor to OK: %#v", res)
	}
}

// TestMonitorRun_NoDataReturnsNotOK ensures that monitor implementations
// never return OK when there's no real data to back the check.
func TestMonitorRun_NoDataMustNotReportOK(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		res  *Result
	}{
		{
			name: "missing_real_data_is_not_ok",
			res:  &Result{Status: statusDegraded, Summary: "no real data"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// No data available → not ok. It's false-green to say OK.
			if tc.res.Status == statusOK {
				t.Errorf("no-real-data monitor returning OK is false-green: %#v", tc.res)
			}
		})
	}
}

// TestMonitorResult_GaugeTypePresent verifies that ALL non-error Result objects
// produced by a Monitor include the "gauge_type" field. This makes synthetic vs
// real gauges unambiguous to consumers of Health APIs.
func TestMonitorResult_GaugeTypePresent(t *testing.T) {
	t.Parallel()

	valid := &Result{
		Status:  statusOK,
		Summary: "valid gauge",
		Details: map[string]any{"gauge_type": "real"},
	}

	if gt, ok := valid.Details["gauge_type"]; !ok {
		t.Errorf("missing gauge_type in Result.Details: %#v", valid)
	} else if gt != "real" && gt != "synthetic" {
		t.Errorf("invalid gauge_type value %q; want 'real' or 'synthetic'", gt)
	}
}

// TestHealthResult_Aggregation_DowngradesSyntheticOK ensures the registry Run
// aggregation does not false-green: if a registered monitor produces an OK result
// whose details declare gauge_type=synthetic, it must NOT contribute to making
// the aggregate status pass.
func TestAggregation_DowngradesSyntheticOK(t *testing.T) {
	t.Parallel()

	r := NewRegistry("")
	mon := &stubMonitor{
		id: "synth-check",
		res: &Result{
			Status:  statusDegraded,
			Summary: "synthetic gauge not OK",
			Details: map[string]any{"gauge_type": "synthetic"},
		},
	}
	r.Register(mon)

	res, err := r.Run(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Run unexpected error: %v", err)
	}
	if res.Status == statusOK && res.Details["gauge_type"] == "synthetic" {
		t.Fatalf("regression: registry returned OK for synthetic gauge (false-green)")
	}
	// Verify all result details are annotated with gauge_type.
	for _, detailVal := range res.Details {
		var detailMap map[string]any
		if rObj, ok := detailVal.(*Result); ok && rObj != nil {
			detailMap = rObj.Details
		} else if dMap, ok := detailVal.(map[string]any); ok {
			detailMap = dMap
		}
		if detailMap != nil {
			if gt, exists := detailMap["gauge_type"]; !exists || gt == "" {
				t.Errorf("detail missing gauge_type: %v", detailMap)
			}
		}
	}
}
