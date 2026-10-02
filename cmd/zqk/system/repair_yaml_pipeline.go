package system

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const pipelineKindRepairYAML = "system_repair_yaml"

// Outcome keys for system_repair_yaml pipeline observability (wire shape unchanged).
const (
	repairYamlOutcomeKeyResolvedFile     = "resolved_file"
	repairYamlOutcomeKeyIngestDone       = "ingest_done"
	repairYamlOutcomeKeyYAMLValid        = "yaml_valid"
	repairYamlOutcomeKeyNormalizeDone    = "normalize_done"
	repairYamlOutcomeKeyDecideDone       = "decide_done"
	repairYamlOutcomeKeyDryRun           = "dry_run"
	repairYamlOutcomeKeyObjectID         = "object_id"
	repairYamlOutcomeKeyRepairKind       = "repair_kind"
	repairYamlOutcomeKeyCommitSkipped    = "commit_skipped"
	repairYamlOutcomeKeyCommitDurationMs = "commit_duration_ms"
	repairYamlOutcomeKeyCommitDone       = "commit_done"
	repairYamlOutcomeKeyFinalizeDone     = "finalize_done"
)

type repairYAMLPipelinePayload struct {
	cmd         *cobra.Command
	projectRoot string

	inputFilePath string
	objectIDArg   string
	kindArg       string
	dryRun        bool

	eventLogger *logging.EventLogger

	// Derived / extracted
	resolvedFilePath string
	yamlValid        bool
	shouldRepair     bool

	instance      map[string]any
	extractedKind string
	repairKind    string
	schemaVersion string
	objectID      string

	// Storage update phase outputs
	verifyReadBack bool
}

func executeRepairYAMLCorrectionCore(
	projectRoot string,
	objectID string,
	instance map[string]any,
	repairKind string,
	schemaVersion string,
	eventLogger *logging.EventLogger,
) error {
	stdctx := pkgctx.NewSystemContext()
	stdctx = pkgctx.WithCacheUpdate(stdctx, objectID, repairKind, "")

	storageFactory, err := storage.NewStorageFactory(stdctx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}
	storageProvider := storageFactory.GetStorage()
	if storageProvider == nil {
		return errfmt.Errorf("storage provider is nil")
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	if err := storageProvider.Update(stdctx, secCtx, objectID, instance); err != nil {
		return errfmt.Newf("failed to update object via storage provider").Wrap(err)
	}

	logging.FluentEvent(eventLogger).Info("Successfully repaired object using storage provider (CAS)").
		ObjectID(objectID).
		Kind(repairKind).
		String("schema_version", schemaVersion).
		Log()

	// Verify the repaired object can be read back
	readObj, readErr := storageProvider.Read(stdctx, secCtx, objectID)
	if readErr != nil {
		logging.FluentEvent(eventLogger).Warn("Failed to read back repaired object for verification").
			ObjectID(objectID).
			WithError(readErr).
			Log()
		return nil
	}

	logging.FluentEvent(eventLogger).Info("Verified: Repaired object can be read back successfully").
		ObjectID(objectID).
		Log()
	_ = readObj // keep for potential future diagnostics
	return nil
}

// RunRepairYAMLViaPipeline wraps `system repair-yaml` in the canonical pipeline lifecycle.
func RunRepairYAMLViaPipeline(
	cmd *cobra.Command,
	projectRoot string,
	filePathArg string,
	idArg string,
	kindArg string,
	dryRun bool,
	eventLogger *logging.EventLogger,
) error {
	if cmd == nil {
		return errfmt.Errorf("repair-yaml: cmd required")
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("repair-yaml: projectRoot required")
	}

	stageLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = pkgctx.NewSystemContext()
	}

	payload := &repairYAMLPipelinePayload{
		cmd:           cmd,
		projectRoot:   projectRoot,
		inputFilePath: filePathArg,
		objectIDArg:   idArg,
		kindArg:       kindArg,
		dryRun:        dryRun,
		eventLogger:   eventLogger,
		shouldRepair:  true,
		schemaVersion: objects.DefaultSchemaVersion,
	}

	pl := pipeline.NewBuilder(pipelineKindRepairYAML, stageLogger).
		WithProfile(string(pkgctx.ProfileSystem)).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopMetricsSink{}, Strategy: pipeline.NoopBucketing{}}).
		AddStage("INGEST", func(pctx *pipeline.Context, in any) (any, error) {
			p, ok := nildecode.DecodeNonNilPayload[*repairYAMLPipelinePayload](in)
			if !ok {
				return nil, errfmt.Errorf("INGEST expected *repairYAMLPayload, got %T", in)
			}

			filePath := p.inputFilePath
			if filePath == emptyValue && p.objectIDArg != emptyValue {
				found, err := findObjectFileByID(p.projectRoot, p.objectIDArg)
				if err != nil {
					return nil, errfmt.Errorf("failed to find file for ID %s: %w", p.objectIDArg, err)
				}
				if found == emptyValue {
					return nil, errfmt.Errorf("file not found for object ID: %s", p.objectIDArg)
				}
				filePath = found
			}
			if filePath == emptyValue {
				return nil, errfmt.Errorf("must specify either --file or --id")
			}

			if !filepath.IsAbs(filePath) {
				filePath = filepath.Join(p.projectRoot, filePath)
			}
			if _, err := fileutil.Stat(filePath); fileutil.IsNotExist(err) {
				return nil, errfmt.Errorf("file not found: %s", filePath)
			}

			p.resolvedFilePath = filePath
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[repairYamlOutcomeKeyResolvedFile] = filePath
			pctx.Outcome[repairYamlOutcomeKeyIngestDone] = true
			return p, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, in any) (any, error) {
			p, ok := nildecode.DecodeNonNilPayload[*repairYAMLPipelinePayload](in)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *repairYAMLPayload, got %T", in)
			}

			data, err := fileutil.ReadFile(p.resolvedFilePath)
			if err != nil {
				return nil, errfmt.Newf("failed to read file").Wrap(err)
			}

			// Quick check: if YAML parses normally, no repair needed.
			var testParse map[string]any
			parseErr := yaml.Unmarshal(data, &testParse)
			if parseErr == nil {
				p.yamlValid = true
				p.shouldRepair = false
				if p.eventLogger != nil {
					logging.FluentEvent(p.eventLogger).Info("File appears to be valid YAML, no repair needed").
						File(p.resolvedFilePath).
						Log()
				}
				return p, nil
			}

			if p.eventLogger != nil {
				logging.FluentEvent(p.eventLogger).Info("File has YAML parsing errors, attempting repair").
					File(p.resolvedFilePath).
					WithError(parseErr).
					Log()
			}

			p.shouldRepair = true
			p.yamlValid = false
			p.instance = nil
			pctx.Outcome[repairYamlOutcomeKeyYAMLValid] = false
			pctx.Outcome[repairYamlOutcomeKeyNormalizeDone] = true
			return p, nil
		}).
		AddStage("DECIDE", func(pctx *pipeline.Context, in any) (any, error) {
			p, ok := nildecode.DecodeNonNilPayload[*repairYAMLPipelinePayload](in)
			if !ok {
				return nil, errfmt.Errorf("DECIDE expected *repairYAMLPayload, got %T", in)
			}

			if !p.shouldRepair {
				pctx.Outcome[repairYamlOutcomeKeyDecideDone] = true
				return p, nil
			}

			if p.dryRun {
				if p.eventLogger != nil {
					logging.FluentEvent(p.eventLogger).Info("DRY RUN: Would repair file").
						File(p.resolvedFilePath).
						Log()
				}
				p.shouldRepair = false
				pctx.Outcome[repairYamlOutcomeKeyDryRun] = true
				pctx.Outcome[repairYamlOutcomeKeyDecideDone] = true
				return p, nil
			}

			// Extract fields from corrupted YAML.
			instance, extractedKind, err := extractFieldsFromCorruptedYAML(p.resolvedFilePath)
			if err != nil {
				return nil, errfmt.Newf("failed to extract fields").Wrap(err)
			}
			p.instance = instance
			p.extractedKind = extractedKind

			repairKind := strings.TrimSpace(p.kindArg)
			if repairKind == emptyValue {
				repairKind = extractedKind
			}
			if repairKind == emptyValue {
				return nil, errfmt.Errorf("could not determine object kind (use --kind to specify)")
			}
			p.repairKind = repairKind

			schemaVersion := objects.DefaultSchemaVersion
			if sv, ok := instance[objects.FieldKeySchemaVersion].(string); ok && sv != emptyValue {
				schemaVersion = sv
			}
			p.schemaVersion = schemaVersion

			objectID, ok := instance[objects.FieldKeyID].(string)
			if !ok || objectID == emptyValue {
				return nil, errfmt.Errorf("instance must have an 'id' field")
			}
			p.objectID = objectID

			pctx.Outcome[repairYamlOutcomeKeyDecideDone] = true
			pctx.Outcome[repairYamlOutcomeKeyObjectID] = objectID
			pctx.Outcome[repairYamlOutcomeKeyRepairKind] = repairKind
			return p, nil
		}).
		AddStage("COMMIT", func(pctx *pipeline.Context, in any) (any, error) {
			p, ok := nildecode.DecodeNonNilPayload[*repairYAMLPipelinePayload](in)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected *repairYAMLPayload, got %T", in)
			}
			if !p.shouldRepair {
				pctx.Outcome[repairYamlOutcomeKeyCommitSkipped] = true
				return p, nil
			}

			if p.eventLogger == nil {
				return nil, errfmt.Errorf("repair-yaml: eventLogger required for commit logging")
			}

			start := time.Now()
			err := executeRepairYAMLCorrectionCore(
				p.projectRoot,
				p.objectID,
				p.instance,
				p.repairKind,
				p.schemaVersion,
				p.eventLogger,
			)
			pctx.Outcome[repairYamlOutcomeKeyCommitDurationMs] = time.Since(start).Milliseconds()

			if err != nil {
				return nil, err
			}
			p.verifyReadBack = true
			pctx.Outcome[repairYamlOutcomeKeyCommitDone] = true
			return p, nil
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, in any) (any, error) {
			p, ok := nildecode.DecodeNonNilPayload[*repairYAMLPipelinePayload](in)
			if !ok {
				return nil, errfmt.Errorf("FINALIZE expected *repairYAMLPayload, got %T", in)
			}
			pctx.Outcome[repairYamlOutcomeKeyFinalizeDone] = true
			return p, nil
		}).
		Build()

	_, err := pl.RunWithContext(baseCtx, payload)
	return err
}
