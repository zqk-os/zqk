package scheduler

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// runCEFDiamondConvergenceMeasure handles evaluation_surface=cef_diamond_scorecard.
// Does not twin SCH-cvs-*-tick / health.jsonl.
func runCEFDiamondConvergenceMeasure(
	cliCtx *cli.Context,
	cmd *cobra.Command,
	projectRoot, sessionID string,
	persistSession bool,
	effCurrentPhase, effFlowVariant string,
	sessionRoutingMeta map[string]any,
	sessionThresholds map[string]any,
) error {
	_ = effCurrentPhase
	_ = effFlowVariant
	res, err := schedpkg.BuildCEFDiamondMeasureResult(projectRoot, sessionID, sessionThresholds)
	if err != nil {
		return err
	}

	var persistOut *persistSessionOutcome
	if persistSession {
		persistOut, err = persistCEFDiamondSession(cliCtx, cmd, sessionID, res)
		if err != nil {
			return err
		}
	}

	format := strings.ToLower(string(cli.GetFormat(cmd)))
	if format == emptyValue {
		format = schedulerFormatTable
	}

	out := map[string]any{
		"evaluation_surface_id":                         res.EvaluationSurfaceID,
		"matrix_name":                                   res.MatrixName,
		"csv_path":                                      res.CSVPath,
		objects.FieldKeyDeltaAssessment:                 res.DeltaAssessment,
		objects.FieldKeyLastMeasurementAt:               res.LastMeasurementAt,
		objects.FieldKeyNextAction:                      res.NextAction,
		objects.FieldKeyPrimaryMeasurementOutcome:       res.PrimaryMeasurementOutcome,
		objects.FieldKeyPrimaryMeasurementOutcomeDetail: res.PrimaryMeasurementOutcomeDetail,
		objects.FieldKeyReadyForSessionCompletion:       res.ReadyForSessionCompletion,
		"session_completion_blocked_reasons":            res.SessionCompletionBlockedReasons,
		objects.FieldKeyAfterStateSnapshot:              res.AfterStateSnapshot,
		"object_update_body":                            res.ObjectUpdateBody,
		objects.FieldKeySessionID:                       sessionID,
		"session_routing_context":                       sessionRoutingMeta,
	}
	if persistOut != nil {
		out["persist_session"] = persistOut
	}

	switch format {
	case schedulerFormatJSON:
		return cli.FormatOutputAs(cmd, cli.FormatJSON, out)
	case schedulerFormatYAML:
		return cli.FormatOutputAs(cmd, cli.FormatYAML, out)
	case schedulerFormatAgentPrompt:
		var b strings.Builder
		b.WriteString("# CEF diamond evaluation surface measure\n\n")
		b.WriteString("evaluation_surface_id: " + res.EvaluationSurfaceID + "\n")
		b.WriteString("delta_assessment: " + res.DeltaAssessment + "\n")
		b.WriteString("primary_measurement_outcome: " + res.PrimaryMeasurementOutcome + "\n")
		b.WriteString("detail: " + res.PrimaryMeasurementOutcomeDetail + "\n")
		b.WriteString("next_action: " + res.NextAction + "\n")
		if persistOut != nil && persistOut.Applied {
			b.WriteString("\n--persist-session: applied CEF after_state_snapshot to the CVS.\n")
		}
		b.WriteString("\n" + agentPromptAutonomyBlock)
		return cli.WriteOutput(cmd, []byte(b.String()))
	default:
		var b strings.Builder
		b.WriteString("CEF diamond convergence measure (evaluation_surface=cef_diamond_scorecard)\n\n")
		b.WriteString("Matrix: " + res.MatrixName + "\n")
		b.WriteString("Delta assessment: " + res.DeltaAssessment + "\n")
		b.WriteString("Primary measurement outcome: " + res.PrimaryMeasurementOutcome + " (" + res.PrimaryMeasurementOutcomeDetail + ")\n")
		b.WriteString("Next action: " + res.NextAction + "\n")
		if persistOut != nil {
			if persistOut.Applied {
				b.WriteString("\n--persist-session: applied.\n")
			} else {
				b.WriteString("\n--persist-session: requested but not applied.\n")
			}
		}
		return cli.WriteOutput(cmd, []byte(b.String()))
	}
}

func persistCEFDiamondSession(cliCtx *cli.Context, cmd *cobra.Command, sessionID string, res *schedpkg.CEFDiamondMeasureResult) (*persistSessionOutcome, error) {
	if res == nil || len(res.ObjectUpdateBody) == 0 {
		return nil, errfmt.Errorf("persist-session: empty CEF object_update_body")
	}
	_ = cliCtx
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return nil, err
	}
	ctx := proc.OperationContext()
	sec := proc.SecurityContext()
	body := res.ObjectUpdateBody
	if err := proc.Storage().Update(ctx, sec, sessionID, body); err != nil {
		return nil, errfmt.Newf("persist-session CEF").Wrap(err)
	}
	return &persistSessionOutcome{Requested: true, Applied: true}, nil
}
