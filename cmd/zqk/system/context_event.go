// context_event.go: append a context_events.jsonl line (pkg/contextevents) for matrix/criteria/manual signals.
package system

import (
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/contextevents"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// NewEmitContextEventCmd emits one structured line to .zqk/metrics/context_events.jsonl (POL-OBS-001).
func NewEmitContextEventCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemEmitContextEventCommandBuilder(), &cobra.Command{Use: "emit-context-event"})
	cmd.Args = cobra.NoArgs
	cli.BindAsyncProgress(cmd, runEmitContextEvent)
	return cmd
}

func runEmitContextEvent(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveCommandProjectRoot(cmd)
	if err != nil {
		return err
	}

	eventType, _ := cmd.Flags().GetString(emitContextEventFlagEventType)
	source, _ := cmd.Flags().GetString(emitContextEventFlagSource)
	corr, _ := cmd.Flags().GetString(emitContextEventFlagCorrelation)
	cvs, _ := cmd.Flags().GetString(emitContextEventFlagCvs)
	jobID, _ := cmd.Flags().GetString(emitContextEventFlagJob)
	criteria, _ := cmd.Flags().GetString(emitContextEventFlagCriteriaRefs)
	bli, _ := cmd.Flags().GetString(emitContextEventFlagBliRefs)
	note, _ := cmd.Flags().GetString(emitContextEventFlagNote)
	payloadRaw, _ := cmd.Flags().GetString(emitContextEventFlagPayloadJSON)

	rec := &contextevents.Record{
		EventType:             strings.TrimSpace(eventType),
		Source:                strings.TrimSpace(source),
		CorrelationID:         strings.TrimSpace(corr),
		ConvergenceSessionRef: strings.TrimSpace(cvs),
		JobID:                 strings.TrimSpace(jobID),
		Note:                  note,
		CriteriaRefs:          splitCommaNonEmpty(criteria),
		BacklogItemRefs:       splitCommaNonEmpty(bli),
	}
	if strings.TrimSpace(payloadRaw) != "" {
		pl, err := contextevents.MergePayloadJSON(nil, []byte(payloadRaw))
		if err != nil {
			return err
		}
		rec.Payload = pl
	}

	if err := contextevents.Append(projectRoot, rec); err != nil {
		if err == contextevents.ErrEmptyEventType {
			return errfmt.Errorf("--%s is required", emitContextEventFlagEventType)
		}
		return err
	}

	outPath := contextevents.ContextEventsJSONLPath(projectRoot)
	return cli.WriteOutput(cmd, []byte(outPath+"\n"))
}

func splitCommaNonEmpty(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
