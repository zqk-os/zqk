package convergerollup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// rollup_status_parity.json is the shared contract with scripts/cvs_outcome_rollup_test.py (BLI parity).

type rollupParityFile struct {
	Cases []rollupParityCase `json:"cases"`
}

type rollupParityCase struct {
	Name       string `json:"name"`
	TB         tbJSON `json:"tb"`
	FK         int    `json:"fk"`
	ZE         int    `json:"ze"`
	MatrixPend *int   `json:"matrix_pending"`
	MatrixErr  string `json:"matrix_err"`
	Children   []struct {
		ID       string   `json:"id"`
		Status   string   `json:"status"`
		Blockers []string `json:"blockers"`
	} `json:"children"`
	WantStatus                       string            `json:"want_status"`
	WantReadyParent                  bool              `json:"want_ready_parent"`
	WantBlockerCodes                 []string          `json:"want_blocker_codes"`
	WantBlockerDetails               map[string]string `json:"want_blocker_details"`
	WantPrimaryMeasurementOutcome    string            `json:"want_primary_measurement_outcome"`
	WantPrimaryMeasurementOutcomeDet string            `json:"want_primary_measurement_outcome_detail"`
}

type tbJSON struct {
	ReadyForSessionCompletion bool     `json:"ready_for_session_completion"`
	FailingFingerprintsNow    []string `json:"failing_fingerprints_now"`
	DeltaAssessment           string   `json:"delta_assessment"`
}

func TestRollupStatusParityJSON(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "rollup_status_parity.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var f rollupParityFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	for _, tc := range f.Cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			tb := TestBundleInput{
				ReadyForSessionCompletion: tc.TB.ReadyForSessionCompletion,
				FailingFingerprintsNow:    tc.TB.FailingFingerprintsNow,
				DeltaAssessment:           tc.TB.DeltaAssessment,
			}
			var kids []ChildSessionInput
			for _, ch := range tc.Children {
				kids = append(kids, ChildSessionInput{
					ID:       ch.ID,
					Status:   ch.Status,
					Blockers: ch.Blockers,
				})
			}
			st, blockers, ready, _ := ComputeRollupStatus(tb, tc.FK, tc.ZE, tc.MatrixPend, tc.MatrixErr, kids)
			if string(st) != tc.WantStatus {
				t.Fatalf("rollup_status = %q want %q", st, tc.WantStatus)
			}
			if ready != tc.WantReadyParent {
				t.Fatalf("ready_for_parent_completion = %v want %v", ready, tc.WantReadyParent)
			}
			got := blockerCodesSorted(blockers)
			want := append([]string(nil), tc.WantBlockerCodes...)
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("blocker codes = %v want %v (full blockers %#v)", got, want, blockers)
			}
			for code, wantDetail := range tc.WantBlockerDetails {
				var gotDetail string
				for _, b := range blockers {
					if strings.TrimSpace(b.Code) == code {
						gotDetail = b.Detail
						break
					}
				}
				if gotDetail != wantDetail {
					t.Fatalf("blocker %q detail = %q want %q", code, gotDetail, wantDetail)
				}
			}
			pm, pmd := ComputePrimaryMeasurementOutcome(st, blockers, strings.TrimSpace(tc.MatrixErr), tc.FK, tc.ZE, false)
			if string(pm) != tc.WantPrimaryMeasurementOutcome {
				t.Fatalf("primary_measurement_outcome = %q want %q", pm, tc.WantPrimaryMeasurementOutcome)
			}
			if pmd != tc.WantPrimaryMeasurementOutcomeDet {
				t.Fatalf("primary_measurement_outcome_detail = %q want %q", pmd, tc.WantPrimaryMeasurementOutcomeDet)
			}
		})
	}
}

func blockerCodesSorted(blockers []Blocker) []string {
	out := make([]string, 0, len(blockers))
	for _, b := range blockers {
		out = append(out, strings.TrimSpace(b.Code))
	}
	return out
}
