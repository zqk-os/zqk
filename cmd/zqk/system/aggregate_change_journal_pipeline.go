package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/storage"
)

const pipelineKindAggregateChangeJournal = "system_aggregate_change_journal"

// Outcome keys for system_aggregate_change_journal pipeline observability (wire shape unchanged).
const (
	aggChangeJournalOutcomeKeyEntriesProcessed   = "entries_processed"
	aggChangeJournalOutcomeKeyMetricsCreated     = "metrics_created"
	aggChangeJournalOutcomeKeyNormalizeCompleted = "normalize_completed"
	aggChangeJournalOutcomeKeyDeleteRequested    = "delete_requested"
	aggChangeJournalOutcomeKeyDecideCompleted    = "decide_completed"
	aggChangeJournalOutcomeKeyCleanupCompleted   = "cleanup_completed"
	aggChangeJournalOutcomeKeyFinalizeDone       = "finalize_done"
)

type aggregateChangeJournalPipelinePayload struct {
	cmd         *cobra.Command
	ctx         *cli.Context
	projectRoot string
	service     *storage.ChangeJournalAggregationService

	windowStart time.Time
	windowEnd   time.Time

	logger     logging.Logger
	secCtx     *pkgctx.SecurityContext
	storageCtx *pkgctx.StorageContext

	deleteRequested bool
	result          *storage.ChangeJournalAggregationResult
}

// RunAggregateChangeJournalViaPipeline wraps `aggregate-change-journal` in the canonical pipeline lifecycle.
func RunAggregateChangeJournalViaPipeline(cmd *cobra.Command, _ []string) error {
	if cmd == nil {
		return errfmt.Errorf("aggregate-change-journal: cmd required")
	}

	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = pkgctx.NewSystemContext()
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	pl := pipeline.NewBuilder(pipelineKindAggregateChangeJournal, logger).
		WithProfile(string(pkgctx.ProfileSystem)).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopMetricsSink{}, Strategy: pipeline.NoopBucketing{}}).
		AddStage("INGEST", func(pctx *pipeline.Context, _ any) (any, error) {
			out := &aggregateChangeJournalPipelinePayload{cmd: cmd}

			ctx := cli.GetContext(cmd)
			if ctx == nil {
				return nil, errfmt.Errorf("aggregate-change-journal: failed to get context")
			}
			out.ctx = ctx

			projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
			if projectRoot == emptyValue {
				return nil, errfmt.Errorf("aggregate-change-journal: project root not found")
			}
			var storageProvider storage.ObjectStorageProvider
			var err error
			if cmd != nil {
				storageProvider, err = getStorageProvider(cmd, projectRoot)
			} else {
				storageProvider, err = getStorageProvider(nil, projectRoot)
			}
			if err != nil {
				return nil, errfmt.Newf("aggregate-change-journal: failed to initialize storage").Wrap(err)
			}

			out.service = storage.NewChangeJournalAggregationService(storageProvider)

			windowStart, windowEnd, err := parseTimeWindow(cmd)
			if err != nil {
				return nil, errfmt.Newf("aggregate-change-journal: failed to parse time window").Wrap(err)
			}
			out.windowStart = windowStart
			out.windowEnd = windowEnd

			profile := systemProfileSystem
			if ctx.Profile != emptyValue {
				profile = ctx.Profile
			}
			out.logger = logging.GetLoggerFromProfile(profile)

			out.secCtx = pkgctx.NewSystemSecurityContext()
			out.storageCtx = pkgctx.NewStorageContext()

			return out, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*aggregateChangeJournalPipelinePayload](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *aggregateChangeJournalPipelinePayload, got %T", payload)
			}

			logging.Fluent(in.logger).Info("Aggregating change journal entries").
				String("start", in.windowStart.Format(time.RFC3339)).
				String("end", in.windowEnd.Format(time.RFC3339)).
				Log()

			result, err := executeAggregateChangeJournalCore(in.service, pkgctx.NewSystemContext(), in.secCtx, in.storageCtx, in.windowStart, in.windowEnd)
			if err != nil {
				return nil, errfmt.Newf("aggregation failed").Wrap(err)
			}
			in.result = result

			// Output results
			var buf strings.Builder
			buf.WriteString("\n✅ Aggregation complete!\n")
			fmt.Fprintf(&buf, "  - Entries processed: %d\n", result.EntryCount)
			fmt.Fprintf(&buf, "  - Metrics created: %d\n", result.MetricsCreated)
			if result.MetricID != emptyValue {
				fmt.Fprintf(&buf, "  - Metric ID: %s\n", result.MetricID)
			}
			fmt.Fprintf(&buf, "  - Entries updated: %d\n", result.EntriesUpdated)

			if err := cli.WriteOutput(in.cmd, []byte(buf.String())); err != nil {
				return nil, err
			}

			pctx.Outcome[aggChangeJournalOutcomeKeyEntriesProcessed] = result.EntryCount
			pctx.Outcome[aggChangeJournalOutcomeKeyMetricsCreated] = result.MetricsCreated
			pctx.Outcome[aggChangeJournalOutcomeKeyNormalizeCompleted] = true
			return in, nil
		}).
		AddStage("DECIDE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*aggregateChangeJournalPipelinePayload](payload)
			if !ok {
				return nil, errfmt.Errorf("DECIDE expected *aggregateChangeJournalPipelinePayload, got %T", payload)
			}

			shouldDelete, _ := in.cmd.Flags().GetBool("delete")
			in.deleteRequested = shouldDelete
			pctx.Outcome[aggChangeJournalOutcomeKeyDeleteRequested] = shouldDelete
			pctx.Outcome[aggChangeJournalOutcomeKeyDecideCompleted] = true
			return in, nil
		}).
		AddStage("COMMIT", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*aggregateChangeJournalPipelinePayload](payload)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected *aggregateChangeJournalPipelinePayload, got %T", payload)
			}

			if in.deleteRequested && in.result != nil && len(in.result.EntriesProcessed) > 0 {
				var buf strings.Builder
				buf.WriteString("\n⚠️  Deleting processed entries...\n")
				fmt.Fprintf(&buf, "  - Processing %d entry ID(s)\n", len(in.result.EntriesProcessed))
				if err := cli.WriteOutput(in.cmd, []byte(buf.String())); err != nil {
					return nil, err
				}

				// Ensure CLI operation context for storage (BulkDelete etc.)
				ctx := storage.WithCLIOperation(pctx.Ctx)
				deleteResult, err := in.service.CleanupAggregatedEntries(ctx, in.secCtx, in.result.EntriesProcessed, false)
				if err != nil {
					return nil, errfmt.Newf("cleanup failed").Wrap(err)
				}

				var doneBuf strings.Builder
				fmt.Fprintf(&doneBuf, "  ✅ Entries deleted: %d\n", deleteResult)
				if err := cli.WriteOutput(in.cmd, []byte(doneBuf.String())); err != nil {
					return nil, err
				}
			}
			pctx.Outcome[aggChangeJournalOutcomeKeyCleanupCompleted] = true
			return in, nil
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			pctx.Outcome[aggChangeJournalOutcomeKeyFinalizeDone] = true
			return payload, nil
		}).
		Build()

	initial := &aggregateChangeJournalPipelinePayload{cmd: cmd}
	pctx := &pipeline.Context{Ctx: baseCtx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, initial)
	return err
}
