package scheduler

import (
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/convergerollup"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/quality"
)

// CEFDiamondMeasureResult is the CEF diamond evaluation-surface payload for convergence measure.
// Distinct from TestBundleConvergenceSnapshot so health.jsonl ticks stay untwinned.
// TRACK: BLI-CVS-EVAL-SURFACE-ADAPTER-001.
type CEFDiamondMeasureResult struct {
	EvaluationSurfaceID             string         `json:"evaluation_surface_id"`
	MatrixName                      string         `json:"matrix_name"`
	CSVPath                         string         `json:"csv_path"`
	DeltaAssessment                 string         `json:"delta_assessment"`
	PrimaryMeasurementOutcome       string         `json:"primary_measurement_outcome"`
	PrimaryMeasurementOutcomeDetail string         `json:"primary_measurement_outcome_detail"`
	LastMeasurementAt               string         `json:"last_measurement_at"`
	NextAction                      string         `json:"next_action"`
	ReadyForSessionCompletion       bool           `json:"ready_for_session_completion"`
	SessionCompletionBlockedReasons []string       `json:"session_completion_blocked_reasons,omitempty"`
	AfterStateSnapshot              map[string]any `json:"after_state_snapshot"`
	ObjectUpdateBody                map[string]any `json:"object_update_body"`
}

const (
	cefMatrixAliasDefault     = EvaluationSurfaceCEFDiamondScorecard
	cefColSeatRole            = "seat_role"
	cefColEnvelopeMin         = "envelope_min"
	cefColPackageComplete     = "package_complete"
	cefColFreezeSHA           = "freeze_sha"
	cefColAssessedAt          = "assessed_at"
	cefColRunID               = "run_id"
	cefSeatRoleEnvelope       = "envelope"
	cefPackageCompletePending = "pending"
	cefPackageCompleteYes     = "yes"
)

// BuildCEFDiamondMeasureResult loads the cef_diamond_scorecard matrix rows for sessionID and
// derives delta / completion gates from envelope_min vs thresholds.min_axis_grade.
func BuildCEFDiamondMeasureResult(projectRoot, sessionID string, thresholds map[string]any) (*CEFDiamondMeasureResult, error) {
	if strings.TrimSpace(projectRoot) == emptyValue {
		return nil, errfmt.Errorf("project root required")
	}
	if strings.TrimSpace(sessionID) == emptyValue {
		return nil, errfmt.Errorf("session-id required for cef_diamond_scorecard measure")
	}
	res, err := quality.ResolveMatrixForCLI(projectRoot, cefMatrixAliasDefault, "", "", "")
	if err != nil {
		return nil, errfmt.Newf("resolve cef_diamond_scorecard matrix").Wrap(err)
	}
	refCol := strings.TrimSpace(res.SessionRefColumn)
	if refCol == emptyValue {
		refCol = "cvs_id"
	}
	rows, err := quality.QueryMatrixCSV(res.CSVPath, res.ProfilePath, res.Alias, nil, "", false, sessionID, refCol, 0, nil)
	if err != nil {
		return nil, errfmt.Newf("query cef_diamond_scorecard rows").Wrap(err)
	}
	minAxis := thresholdMinAxisGrade(thresholds)
	return cefMeasureFromRows(res.Alias, res.CSVPath, rows.Rows, minAxis)
}

func thresholdMinAxisGrade(thresholds map[string]any) int {
	const defaultMin = 4
	if thresholds == nil {
		return defaultMin
	}
	raw, ok := thresholds["min_axis_grade"]
	if !ok || raw == nil {
		return defaultMin
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return n
		}
	}
	return defaultMin
}

func cefMeasureFromRows(matrixName, csvPath string, rows []map[string]string, minAxis int) (*CEFDiamondMeasureResult, error) {
	var (
		lastCompleteEnv int
		lastCompleteSHA string
		lastCompleteAt  string
		haveComplete    bool
		pendingCount    int
		inflightSHA     string
		inflightSeats   []string
		matrixRowTotal  = len(rows)
		matrixFullyDone int
	)
	for _, row := range rows {
		pkg := strings.ToLower(strings.TrimSpace(row[cefColPackageComplete]))
		role := strings.ToLower(strings.TrimSpace(row[cefColSeatRole]))
		if pkg == cefPackageCompleteYes || pkg == "na" {
			matrixFullyDone++
		}
		if pkg == cefPackageCompletePending {
			pendingCount++
			if inflightSHA == emptyValue {
				inflightSHA = strings.TrimSpace(row[cefColFreezeSHA])
			}
			if role != emptyValue && role != cefSeatRoleEnvelope {
				seat := strings.TrimSpace(row["agent_run_id"])
				if seat == emptyValue {
					seat = strings.TrimSpace(row[cefColRunID])
				}
				if seat != emptyValue {
					inflightSeats = append(inflightSeats, seat)
				}
			}
			continue
		}
		if role != cefSeatRoleEnvelope {
			continue
		}
		if pkg != cefPackageCompleteYes {
			continue
		}
		envStr := strings.TrimSpace(row[cefColEnvelopeMin])
		if envStr == emptyValue {
			continue
		}
		env, err := strconv.Atoi(envStr)
		if err != nil {
			continue
		}
		assessed := strings.TrimSpace(row[cefColAssessedAt])
		if !haveComplete || assessed >= lastCompleteAt {
			haveComplete = true
			lastCompleteEnv = env
			lastCompleteAt = assessed
			lastCompleteSHA = strings.TrimSpace(row[cefColFreezeSHA])
		}
	}

	blocked := make([]string, 0, 4)
	ready := false
	delta := "unknown"
	outcome := string(convergerollup.MeasurementYieldsAmbiguousOutcome)
	detail := "no complete envelope row for session"
	next := "Fill CEF diamond matrix rows (dual-seat remesure) then re-run convergence measure"

	switch {
	case pendingCount > 0:
		blocked = append(blocked, "dual_seat_remesure_in_flight")
		if haveComplete && lastCompleteEnv < minAxis {
			blocked = append(blocked, "envelope_min_below_"+strconv.Itoa(minAxis))
			delta = "trending_toward"
			outcome = string(convergerollup.MeasurementYieldsDivergence)
			detail = "inflight remesure; last complete envelope_min below floor"
		} else {
			delta = "neutral"
			outcome = string(convergerollup.MeasurementYieldsAmbiguousOutcome)
			detail = "inflight remesure pending package_complete"
		}
		next = "Complete dual-seat CEF package for inflight freeze, objectify residuals, remesure"
	case haveComplete && lastCompleteEnv >= minAxis:
		ready = true
		delta = "trending_toward"
		outcome = string(convergerollup.MeasurementYieldsConvergence)
		detail = "envelope_min meets min_axis_grade"
		next = "Verify desired_end_state axes; VDS evaluate before session completion"
	case haveComplete:
		blocked = append(blocked, "envelope_min_below_"+strconv.Itoa(minAxis))
		delta = "trending_away"
		outcome = string(convergerollup.MeasurementYieldsDivergence)
		detail = "last complete envelope_min below floor"
		next = "Remediate weakest diamond axes; remesure dual-seat until envelope_min>=" + strconv.Itoa(minAxis)
	default:
		blocked = append(blocked, "no_complete_envelope_row")
	}

	after := map[string]any{
		convSugKeyEvaluationSurface:               EvaluationSurfaceCEFDiamondScorecard,
		"evaluation_surface_id":                   EvaluationSurfaceCEFDiamondScorecard,
		"matrix_name":                             matrixName,
		"matrix_row_total":                        matrixRowTotal,
		"matrix_fully_done_rows":                  matrixFullyDone,
		"package_complete_pending":                pendingCount,
		objects.FieldKeyReadyForSessionCompletion: ready,
		convSugKeySessionCompletionBlockedReasons: blocked,
		objects.FieldKeyNote:                      "Out of scope: scheduler test-bundle health.jsonl / failing fingerprints.",
	}
	if haveComplete {
		after["last_complete_envelope_min"] = lastCompleteEnv
		after["last_complete_freeze_sha"] = lastCompleteSHA
		after["last_complete_assessed_at"] = lastCompleteAt
	}
	if pendingCount > 0 {
		after["inflight_freeze_sha"] = inflightSHA
		if len(inflightSeats) > 0 {
			after["inflight_seats"] = inflightSeats
		}
	}
	after[objects.FieldKeyPrimaryMeasurementOutcome] = outcome
	after[objects.FieldKeyPrimaryMeasurementOutcomeDetail] = detail

	measuredAt := lastCompleteAt
	if measuredAt == emptyValue && pendingCount > 0 {
		measuredAt = "inflight"
	}

	body := map[string]any{
		objects.FieldKeyDeltaAssessment:    delta,
		objects.FieldKeyLastMeasurementAt:  measuredAt,
		objects.FieldKeyAfterStateSnapshot: after,
		objects.FieldKeyNextAction:         next,
	}

	return &CEFDiamondMeasureResult{
		EvaluationSurfaceID:             EvaluationSurfaceCEFDiamondScorecard,
		MatrixName:                      matrixName,
		CSVPath:                         csvPath,
		DeltaAssessment:                 delta,
		PrimaryMeasurementOutcome:       outcome,
		PrimaryMeasurementOutcomeDetail: detail,
		LastMeasurementAt:               measuredAt,
		NextAction:                      next,
		ReadyForSessionCompletion:       ready,
		SessionCompletionBlockedReasons: blocked,
		AfterStateSnapshot:              after,
		ObjectUpdateBody:                body,
	}, nil
}
