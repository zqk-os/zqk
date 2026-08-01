package system

import (
	"bufio"
	stdcontext "context"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

const pipelineKindRepairCASCorruption = "system_repair_cas_corruption"

// Outcome keys for system_repair_cas_corruption pipeline observability (wire shape unchanged).
const (
	repairCASOutcomeKeyInputFile      = "input_file"
	repairCASOutcomeKeyQuarantineDir  = "quarantine_dir"
	repairCASOutcomeKeyDryRun         = "dry_run"
	repairCASOutcomeKeyIngestDone     = "ingest_done"
	repairCASOutcomeKeyProcessedLines = "processed_lines"
	repairCASOutcomeKeyParsedRows     = "parsed_rows"
	repairCASOutcomeKeyParseFailed    = "parse_failed"
	repairCASOutcomeKeyNormalizeDone  = "normalize_done"
	repairCASOutcomeKeyCommitRows     = "commit_rows"
	repairCASOutcomeKeyDecideDone     = "decide_done"
	repairCASOutcomeKeyRepairFailed   = "repair_failed"
	repairCASOutcomeKeyCommitDone     = "commit_done"
	repairCASOutcomeKeyFinalizeDone   = "finalize_done"
)

type repairCASCorruptionPipelinePayload struct {
	cmd *cobra.Command

	projectRoot   string
	inputFile     string
	quarantineDir string
	dryRun        bool

	stopScheduler bool
	restartOnExit bool
	eventLogger   *logging.EventLogger

	rows []casCorruptionTSVRow

	processed    int
	parseFailed  int
	repairFailed int
	failed       int
	fixed        int
	skipped      int

	startTime time.Time
}

func executeRepairCASCorruptionCore(
	ctx stdcontext.Context,
	projectRoot string,
	rows []casCorruptionTSVRow,
	dryRun bool,
	quarantineDir string,
	logger *logging.EventLogger,
) (fixed, skipped, repairFailed int) {
	start := time.Now()
	if logger != nil {
		logging.FluentEvent(logger).Info("CAS corruption repair starting").
			Int("rows", len(rows)).
			Bool("dry_run", dryRun).
			Log()
	}

	for i, row := range rows {
		if logger != nil && (i+1)%10 == 0 {
			logging.FluentEvent(logger).Info("repair-cas-corruption progress").
				Int("attempted", i+1).
				String("elapsed", time.Since(start).String()).
				Bool("dry_run", dryRun).
				Log()
		}

		res, err := storage.RepairCASHFilenameMismatch(ctx, projectRoot, row.Kind, row.ObjectID, row.FilePath, &storage.CASCorruptionRepairOptions{
			DryRun:        dryRun,
			QuarantineDir: quarantineDir,
			Logger:        nil, // preserve existing behavior: repair logs are controlled by caller
		})
		if err != nil {
			repairFailed++
			if logger != nil {
				logging.FluentEvent(logger).Warn("Failed to repair CAS corruption").
					Kind(row.Kind).
					ObjectID(row.ObjectID).
					String("file", row.FilePath).
					WithError(err).
					Log()
			}
			continue
		}

		if res.Fixed || dryRun {
			fixed++
		} else {
			skipped++
		}
	}

	return fixed, skipped, repairFailed
}

// RunRepairCASCorruptionViaPipeline wraps `system repair-cas-corruption` in the canonical pipeline lifecycle.
func RunRepairCASCorruptionViaPipeline(
	cmd *cobra.Command,
	projectRoot string,
	inputFile string,
	dryRun bool,
	quarantineDir string,
	stopScheduler bool,
	restartOnExit bool,
	eventLogger *logging.EventLogger,
) error {
	if cmd == nil {
		return errfmt.Errorf("repair-cas-corruption: cmd required")
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("repair-cas-corruption: projectRoot required")
	}

	stageLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	if stopScheduler {
		_ = runSchedulerStopBestEffort(projectRoot)
		if restartOnExit {
			defer func() { _ = runSchedulerStartBestEffort(projectRoot) }()
		}
	}

	payload := &repairCASCorruptionPipelinePayload{
		cmd:           cmd,
		projectRoot:   projectRoot,
		inputFile:     inputFile,
		quarantineDir: quarantineDir,
		dryRun:        dryRun,
		stopScheduler: stopScheduler,
		restartOnExit: restartOnExit,
		eventLogger:   eventLogger,
		startTime:     time.Now(),
	}

	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = pkgctx.NewSystemContext()
	}

	pl := pipeline.NewBuilder(pipelineKindRepairCASCorruption, stageLogger).
		WithProfile(string(pkgctx.ProfileSystem)).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopMetricsSink{}, Strategy: pipeline.NoopBucketing{}}).
		AddStage("INGEST", func(pctx *pipeline.Context, in any) (any, error) {
			p, ok := nildecode.DecodeNonNilPayload[*repairCASCorruptionPipelinePayload](in)
			if !ok {
				return nil, errfmt.Errorf("INGEST expected *repairCASCorruptionPipelinePayload, got %T", in)
			}

			// Resolve defaults.
			if p.inputFile == emptyValue {
				p.inputFile = filepath.Join(p.projectRoot, paths.ProjectDataDir, "system-health", "tier1-cas-corruption.tsv")
			}
			if !filepath.IsAbs(p.inputFile) {
				p.inputFile = filepath.Join(p.projectRoot, p.inputFile)
			}

			if p.quarantineDir == emptyValue {
				p.quarantineDir = filepath.Join(p.projectRoot, paths.ProjectDataDir, paths.SystemHealthDir, paths.QuarantineDir)
			}
			if !filepath.IsAbs(p.quarantineDir) {
				p.quarantineDir = filepath.Join(p.projectRoot, p.quarantineDir)
			}

			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[repairCASOutcomeKeyInputFile] = p.inputFile
			pctx.Outcome[repairCASOutcomeKeyQuarantineDir] = p.quarantineDir
			pctx.Outcome[repairCASOutcomeKeyDryRun] = p.dryRun
			pctx.Outcome[repairCASOutcomeKeyIngestDone] = true
			return p, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, in any) (any, error) {
			p, ok := nildecode.DecodeNonNilPayload[*repairCASCorruptionPipelinePayload](in)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *repairCASCorruptionPipelinePayload, got %T", in)
			}

			f, err := os.Open(p.inputFile)
			if err != nil {
				return nil, errfmt.Newf("repair-cas-corruption: failed to open input file").Wrap(err)
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			// TSV lines can be large (long absolute paths + messages).
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

			lineNo := 0
			for scanner.Scan() {
				lineNo++
				line := strings.TrimSpace(scanner.Text())
				if lineNo == 1 && isRepairCASCorruptionTSVHeader(line) {
					continue
				}
				if line == emptyValue {
					continue
				}

				p.processed++
				row, ok := parseRepairCASCorruptionTSVLine(line)
				if !ok {
					p.parseFailed++
					continue
				}
				if !filepath.IsAbs(row.FilePath) {
					row.FilePath = filepath.Join(p.projectRoot, row.FilePath)
				}
				p.rows = append(p.rows, row)
			}

			if err := scanner.Err(); err != nil {
				return nil, errfmt.Newf("repair-cas-corruption: failed to read input file").Wrap(err)
			}

			p.failed = p.parseFailed // initialize total failed with parse failures

			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[repairCASOutcomeKeyProcessedLines] = p.processed
			pctx.Outcome[repairCASOutcomeKeyParsedRows] = len(p.rows)
			pctx.Outcome[repairCASOutcomeKeyParseFailed] = p.parseFailed
			pctx.Outcome[repairCASOutcomeKeyNormalizeDone] = true
			return p, nil
		}).
		AddStage("DECIDE", func(pctx *pipeline.Context, in any) (any, error) {
			p, ok := nildecode.DecodeNonNilPayload[*repairCASCorruptionPipelinePayload](in)
			if !ok {
				return nil, errfmt.Errorf("DECIDE expected *repairCASCorruptionPipelinePayload, got %T", in)
			}

			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[repairCASOutcomeKeyCommitRows] = len(p.rows)
			pctx.Outcome[repairCASOutcomeKeyDryRun] = p.dryRun
			pctx.Outcome[repairCASOutcomeKeyDecideDone] = true
			return p, nil
		}).
		AddStage("COMMIT", func(pctx *pipeline.Context, in any) (any, error) {
			p, ok := nildecode.DecodeNonNilPayload[*repairCASCorruptionPipelinePayload](in)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected *repairCASCorruptionPipelinePayload, got %T", in)
			}

			ctx := pkgctx.NewSystemContext()
			fixed, skipped, repairFailed := executeRepairCASCorruptionCore(
				ctx,
				p.projectRoot,
				p.rows,
				p.dryRun,
				p.quarantineDir,
				p.eventLogger,
			)

			p.fixed = fixed
			p.skipped = skipped
			p.repairFailed = repairFailed
			p.failed = p.parseFailed + p.repairFailed

			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[objects.FieldKeyFixed] = p.fixed
			pctx.Outcome[objects.FieldKeySkipped] = p.skipped
			pctx.Outcome[repairCASOutcomeKeyRepairFailed] = p.repairFailed
			pctx.Outcome[repairCASOutcomeKeyCommitDone] = true
			return p, nil
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, in any) (any, error) {
			p, ok := nildecode.DecodeNonNilPayload[*repairCASCorruptionPipelinePayload](in)
			if !ok {
				return nil, errfmt.Errorf("FINALIZE expected *repairCASCorruptionPipelinePayload, got %T", in)
			}

			if p.eventLogger != nil {
				logging.FluentEvent(p.eventLogger).Info("CAS corruption repair completed").
					Int("processed", p.processed).
					Int("fixed", p.fixed).
					Int("skipped", p.skipped).
					Int("failed", p.failed).
					Bool("dry_run", p.dryRun).
					String("elapsed", time.Since(p.startTime).String()).
					Log()
			}

			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[repairCASOutcomeKeyFinalizeDone] = true
			return p, nil
		}).
		Build()

	pctx := &pipeline.Context{Ctx: baseCtx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, payload)
	return err
}
