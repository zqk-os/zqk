package utility

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lanceman/zqk/cmd/zqk/system"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
)

const pipelineKindScenarioBuilder = "scenario_builder_data_file"

type scenarioBuilderPayload struct {
	sb                  *ScenarioBuilder
	ctx                 context.Context
	referenceCache      map[string]bool
	referenceCacheMu    *sync.Mutex
	idStream            map[string]string
	idStreamMu          *sync.Mutex
	activeValidations   *sync.WaitGroup
	validationCounter   int
	validationCounterMu sync.Mutex
	created             int
	skipped             int
}

// BuildFromDataFileViaPipeline implements the pipeline pattern for building from a data file.
func (sb *ScenarioBuilder) BuildFromDataFileViaPipeline(ctx context.Context) error {
	var logger logging.Logger
	if sb.logger != nil {
		logger = sb.logger.Logger()
	} else {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}

	pl := pipeline.NewBuilder(pipelineKindScenarioBuilder, logger).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", stageScenarioIngest).
		AddStage("SETUP_INFRASTRUCTURE", stageScenarioSetupInfrastructure).
		AddStage("PREPARE_CACHE", stageScenarioPrepareCache).
		AddStage("CREATE_OBJECTS", stageScenarioCreateObjects).
		AddStage("FINALIZE", stageScenarioFinalize).
		Build()

	payload := &scenarioBuilderPayload{
		sb:                sb,
		ctx:               ctx,
		referenceCache:    make(map[string]bool),
		referenceCacheMu:  &sync.Mutex{},
		idStream:          make(map[string]string),
		idStreamMu:        &sync.Mutex{},
		activeValidations: &sync.WaitGroup{},
	}

	_, err := pl.Run(pctx, payload)
	return err
}

func stageScenarioIngest(stageCtx *pipeline.Context, p any) (any, error) {
	stageCtx.Outcome[pipeline.OutcomeKeyIngestStageStarted] = true
	payload, ok := nildecode.DecodeNonNilPayload[*scenarioBuilderPayload](p)
	if !ok {
		return nil, errfmt.Errorf("expected *scenarioBuilderPayload")
	}

	sb := payload.sb
	if len(sb.dataFileObjects) == 0 {
		return nil, errfmt.Errorf("no objects loaded from data file")
	}

	sb.emitCoordinatorEvent(payload.ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		"Building scenario from data file",
		map[string]any{objects.FieldKeyObjectCount: len(sb.dataFileObjects)})

	return payload, nil
}

func stageScenarioSetupInfrastructure(stageCtx *pipeline.Context, p any) (any, error) {
	payload := p.(*scenarioBuilderPayload)
	sb := payload.sb

	if err := fileutil.MkdirAll(sb.config.TargetDir, paths.DirPerm755); err != nil {
		sb.emitCoordinatorEvent(payload.ctx, ScenarioBuilderProfileName, scenarioBuilderStatusError,
			fmt.Sprintf("Failed to create target directory: %s", err),
			map[string]any{"error": err.Error()})
		return nil, errfmt.Newf("failed to create target directory").Wrap(err)
	}

	sb.emitCoordinatorEvent(payload.ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress, "Setting up infrastructure...", nil)
	if err := sb.setupInfrastructure(); err != nil {
		sb.emitCoordinatorEvent(payload.ctx, ScenarioBuilderProfileName, scenarioBuilderStatusError,
			fmt.Sprintf("Failed to setup infrastructure: %s", err),
			map[string]any{"error": err.Error()})
		return nil, errfmt.Newf("failed to setup infrastructure").Wrap(err)
	}

	return payload, nil
}

func stageScenarioPrepareCache(stageCtx *pipeline.Context, p any) (any, error) {
	payload := p.(*scenarioBuilderPayload)
	sb := payload.sb

	for _, obj := range sb.dataFileObjects {
		if objID, ok := obj[objects.FieldKeyID].(string); ok && objID != emptyValue {
			withLock(payload.referenceCacheMu, func() {
				payload.referenceCache[objID] = true
			})
		}
		if kind, ok := obj[objects.FieldKeyKind].(string); ok && kind == objects.KindAccount {
			if username, ok := obj[objects.FieldKeyUsername].(string); ok && username != emptyValue {
				accountID := "" // ACC-* from storage; TRACK: BLI-1785905134201010000-07393484
				withLock(payload.referenceCacheMu, func() {
					payload.referenceCache[accountID] = true
				})
			}
		}
	}

	objectIDCache := system.GetGlobalObjectIDCache()
	storagepkg.SetCacheChecker(func(objectID string) (string, bool) {
		var actualID string
		var idStreamExists bool
		withLock(payload.idStreamMu, func() {
			actualID, idStreamExists = payload.idStream[objectID]
		})
		if idStreamExists {
			if entry, cacheExists := objectIDCache.Get(actualID); cacheExists && entry != nil && entry.FilePath != emptyValue {
				return entry.FilePath, true
			}
			if kind := validation.GetIDValidator().InferKindFromID(actualID); kind != emptyValue {
				filePath := sb.reconstructFilePath(actualID, kind)
				if filePath != emptyValue {
					return filePath, true
				}
			}
			return "", true
		}

		var refCacheExists, cached bool
		withLock(payload.referenceCacheMu, func() {
			refCacheExists, cached = payload.referenceCache[objectID]
		})
		if cached && refCacheExists {
			if entry, cacheExists := objectIDCache.Get(objectID); cacheExists && entry != nil && entry.FilePath != emptyValue {
				return entry.FilePath, true
			}
			if kind := validation.GetIDValidator().InferKindFromID(objectID); kind != emptyValue {
				if kind == objects.KindAccount && strings.HasPrefix(objectID, "account:") {
					username := strings.TrimPrefix(objectID, "account:")
					dirName := objects.GetDirectoryFromKind(kind)
					if dirName != emptyValue && sb.config != nil && sb.config.TargetDir != emptyValue {
						filename := fmt.Sprintf("account-%s.yaml", username)
						filePath := filepath.Join(datacell.CellCASPrimaryDir(sb.config.TargetDir, dirName), filename)
						return filePath, true
					}
				}
				filePath := sb.reconstructFilePath(objectID, kind)
				if filePath != emptyValue {
					return filePath, true
				}
			}
			return "", true
		}

		if entry, cacheExists := objectIDCache.Get(objectID); cacheExists && entry != nil && entry.FilePath != emptyValue {
			return entry.FilePath, true
		}

		return "", false
	})

	return payload, nil
}

func stageScenarioCreateObjects(stageCtx *pipeline.Context, p any) (any, error) {
	payload := p.(*scenarioBuilderPayload)
	sb := payload.sb
	ctx := payload.ctx

	totalObjects := len(sb.dataFileObjects)
	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		"Creating objects from data file (breadth-first, parallel within layers)...",
		map[string]any{"total_objects": totalObjects, "total_layers": len(sb.dataFileObjectLayers)})

	layers := sb.dataFileObjectLayers
	if len(layers) == 0 {
		layers = [][]map[string]any{sb.dataFileObjects}
	}

	for layerIndex, layer := range layers {
		if len(layer) == 0 {
			continue
		}

		sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			fmt.Sprintf("Processing layer %d/%d (%d objects)...", layerIndex+1, len(layers), len(layer)),
			map[string]any{objects.FieldKeyLayer: layerIndex + 1, "total_layers": len(layers), "objects_in_layer": len(layer)})

		var wg sync.WaitGroup
		layerCreated := 0
		layerSkipped := 0
		layerCreatedMu := &sync.Mutex{}
		layerSkippedMu := &sync.Mutex{}

		for objIndex, obj := range layer {
			objCopy := obj
			objIndexCopy := objIndex
			layerIndexCopy := layerIndex
			kind, _ := obj[objects.FieldKeyKind].(string)
			objID, _ := obj[objects.FieldKeyID].(string)

			goroutineName := fmt.Sprintf("scenario_builder_create_%s_%d", kind, objIndexCopy)
			goroutinePurpose := fmt.Sprintf("creating %s object %s in layer %d", kind, objID, layerIndexCopy+1)

			state := &layerProcessingState{
				referenceCache:      payload.referenceCache,
				referenceCacheMu:    payload.referenceCacheMu,
				idStream:            payload.idStream,
				idStreamMu:          payload.idStreamMu,
				activeValidations:   payload.activeValidations,
				validationCounter:   &payload.validationCounter,
				validationCounterMu: &payload.validationCounterMu,
			}

			goroutinelabels.NewGoroutine(goroutineName, goroutinePurpose).
				WithWaitGroup(&wg).
				WithPanicHandler(func(r any) {
					panicStr := fmt.Sprintf("%v", r)
					if strings.Contains(panicStr, "negative WaitGroup counter") {
						sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
							fmt.Sprintf("WaitGroup panic recovered: %v", r),
							map[string]any{
								objects.FieldKeyKind:  kind,
								objects.FieldKeyLayer: layerIndex + 1,
								"object_index":        objIndexCopy,
								objects.FieldKeyID:    objID,
								"panic":               panicStr,
								objects.FieldKeyNote:  "This panic is being handled gracefully",
							})
					} else {
						sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusError,
							fmt.Sprintf("Panic in goroutine: %v", r),
							map[string]any{
								objects.FieldKeyKind:  kind,
								objects.FieldKeyLayer: layerIndex + 1,
								"object_index":        objIndexCopy,
								objects.FieldKeyID:    objID,
								"panic":               panicStr,
							})
					}
					withLock(layerSkippedMu, func() { layerSkipped++ })
				}).
				WithErrorHandler(func(err error) {
					if err != nil {
						withLock(layerSkippedMu, func() { layerSkipped++ })
					}
				}).
				Start(func() error {
					return sb.createObjectInLayer(ctx, objCopy, objIndexCopy, layerIndexCopy, state, layerCreatedMu, layerSkippedMu, &layerCreated)
				})
		}

		wg.Wait()

		payload.created += layerCreated
		payload.skipped += layerSkipped

		sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			fmt.Sprintf("Layer %d/%d complete: %d created, %d skipped", layerIndex+1, len(layers), layerCreated, layerSkipped),
			map[string]any{
				objects.FieldKeyLayer:   layerIndex + 1,
				"total_layers":          len(layers),
				"created":               layerCreated,
				objects.FieldKeySkipped: layerSkipped,
			})
	}

	if len(layers) == 0 || (len(layers) == 1 && len(layers[0]) == 0 && len(sb.dataFileObjects) > 0) {
		state := &layerProcessingState{
			referenceCache:      payload.referenceCache,
			referenceCacheMu:    payload.referenceCacheMu,
			idStream:            payload.idStream,
			idStreamMu:          payload.idStreamMu,
			activeValidations:   payload.activeValidations,
			validationCounter:   &payload.validationCounter,
			validationCounterMu: &payload.validationCounterMu,
		}
		sequentialCreated, sequentialSkipped := sb.processObjectsSequentially(ctx, state)
		payload.created += sequentialCreated
		payload.skipped += sequentialSkipped
	}

	return payload, nil
}

func stageScenarioFinalize(stageCtx *pipeline.Context, p any) (any, error) {
	payload := p.(*scenarioBuilderPayload)
	sb := payload.sb
	ctx := payload.ctx

	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		"Waiting for all validations to complete before clearing cache checker",
		map[string]any{
			"total_objects":         len(sb.dataFileObjects),
			"created":               payload.created,
			objects.FieldKeySkipped: payload.skipped,
		})
	payload.activeValidations.Wait()

	withLock(&payload.validationCounterMu, func() {
		if payload.validationCounter != 0 {
			sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
				fmt.Sprintf("Validation counter is %d (expected 0)", payload.validationCounter),
				map[string]any{"validation_counter": payload.validationCounter})
		}
	})

	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		"Clearing cache checker after all objects created and all validations complete",
		map[string]any{"total_objects": len(sb.dataFileObjects), "created": payload.created, objects.FieldKeySkipped: payload.skipped})
	storagepkg.SetCacheChecker(nil)

	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		fmt.Sprintf("Created %d objects (%d skipped)", payload.created, payload.skipped),
		map[string]any{"created": payload.created, objects.FieldKeySkipped: payload.skipped, "total": len(sb.dataFileObjects)})

	totalGenerated := 0
	for kind, count := range sb.generated {
		totalGenerated += count
		sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			fmt.Sprintf("Created %d %s objects", count, kind),
			map[string]any{objects.FieldKeyKind: kind, "count": count})
	}

	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusComplete,
		fmt.Sprintf("Scenario build complete: %d total objects across %d kinds", totalGenerated, len(sb.generated)),
		map[string]any{"total_objects": totalGenerated, "total_kinds": len(sb.generated)})

	objectIDCache := system.GetGlobalObjectIDCache()
	if err := objectIDCache.SaveCache(sb.config.TargetDir); err != nil {
		sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
			"Failed to save object ID cache after scenario build",
			map[string]any{
				"project_root": sb.config.TargetDir,
				"error":        err,
			})
	}

	return payload, nil
}
