package system

import (
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

// HierarchicalFixBatch processes fixes in dependency-ordered batches
// Each dependency layer is processed in parallel (with goroutines)
// But layers are processed sequentially to respect dependency hierarchy
type HierarchicalFixBatch struct {
	projectRoot     string
	maxBatchSize    int // Maximum objects per batch (default: 1000, configurable)
	specLoader      *objects.SpecLoader
	storageProvider storage.ObjectStorageProvider
	logger          logging.Logger
}

// NewHierarchicalFixBatch creates a new hierarchical fix batch processor
func NewHierarchicalFixBatch(
	projectRoot string,
	maxBatchSize int,
	specLoader *objects.SpecLoader,
	storageProvider storage.ObjectStorageProvider,
	logger logging.Logger,
) *HierarchicalFixBatch {
	if maxBatchSize <= 0 {
		maxBatchSize = 1000 // Default limit
	}

	return &HierarchicalFixBatch{
		projectRoot:     projectRoot,
		maxBatchSize:    maxBatchSize,
		specLoader:      specLoader,
		storageProvider: storageProvider,
		logger:          logger,
	}
}

// FixBatchResult represents the result of processing a batch of fixes
type FixBatchResult struct {
	Processed int
	Fixed     int
	Failed    int
	Errors    []string
}

// ProcessHierarchicalFixes processes issues grouped by dependency priority
// Issues are sorted into dependency layers and processed sequentially per layer
// Within each layer, fixes are processed in parallel (via goroutines)
func (hfb *HierarchicalFixBatch) ProcessHierarchicalFixes(
	ctx *cli.Context,
	issuesByObject map[string][]Issue, // objectID -> issues
) (*FixBatchResult, error) {
	result := &FixBatchResult{
		Errors: []string{},
	}

	// Group issues by dependency priority (layer)
	issuesByLayer := make(map[int][]FixTask)
	for objID, issues := range issuesByObject {
		for _, issue := range issues {
			priority := getFixDependencyPriority(issue)
			task := FixTask{
				ObjectID: objID,
				Issue:    issue,
				Priority: priority,
			}
			issuesByLayer[priority] = append(issuesByLayer[priority], task)
		}
	}

	// Process layers in dependency order (highest priority first)
	// Priority values: 4 (goals) > 3 (priority_plans) > 2 (milestones) > 1 (backlog_items)
	priorities := []int{4, 3, 2, 1, 0} // 0 means unknown/other

	for _, priority := range priorities {
		tasks := issuesByLayer[priority]
		if len(tasks) == 0 {
			continue
		}

		logging.Fluent(hfb.logger).Debug("Processing fix layer").
			Int("priority", priority).
			Int("task_count", len(tasks)).
			Log()

		// Process this layer in batches (respecting maxBatchSize)
		for batchStart := 0; batchStart < len(tasks); batchStart += hfb.maxBatchSize {
			batchEnd := batchStart + hfb.maxBatchSize
			if batchEnd > len(tasks) {
				batchEnd = len(tasks)
			}
			batch := tasks[batchStart:batchEnd]

			layerResult := hfb.processLayerBatch(ctx, batch)
			result.Processed += layerResult.Processed
			result.Fixed += layerResult.Fixed
			result.Failed += layerResult.Failed
			result.Errors = append(result.Errors, layerResult.Errors...)
		}
	}

	return result, nil
}

// FixTask represents a single fix task with its dependency priority
type FixTask struct {
	ObjectID string
	Issue    Issue
	Priority int
}

// processLayerBatch processes a batch of fixes from the same dependency layer in parallel
func (hfb *HierarchicalFixBatch) processLayerBatch(
	ctx *cli.Context,
	tasks []FixTask,
) *FixBatchResult {
	result := &FixBatchResult{
		Errors: []string{},
	}

	if len(tasks) == 0 {
		return result
	}

	// Process fixes in parallel within this layer
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, task := range tasks {
		t := task // Capture loop variable
		goroutinelabels.NewGoroutine(fmt.Sprintf("hierarchical_fix_%s", t.ObjectID), fmt.Sprintf("applying hierarchical fix for object %s", t.ObjectID)).
			WithWaitGroup(&wg).
			StartSimple(func() {

				// Create fix context for this task
				// Note: We need to read the object first to create a proper fix context
				stdctx := pkgctx.NewSystemContext()
				secCtx := pkgctx.NewSystemSecurityContext()
				obj, readErr := hfb.storageProvider.Read(stdctx, secCtx, t.ObjectID)
				if readErr != nil {
					mu.Lock()
					result.Failed++
					result.Errors = append(result.Errors, fmt.Sprintf("Failed to read object %s: %v", t.ObjectID, readErr))
					mu.Unlock()
					return
				}

				// Create a minimal ParsedObject for fix context
				parsedObj := &parser.ParsedObject{
					ID:         t.ObjectID,
					Kind:       getKindFromObject(obj),
					Properties: obj,
				}

				// Get file path for object (optional, may not be needed for fix command execution)
				filePath, _ := findObjectByID(hfb.projectRoot, t.ObjectID)

				// Create AutoFixContext
				fixCtx := &AutoFixContext{
					Ctx:               ctx,
					Obj:               parsedObj,
					FilePath:          filePath,
					Kind:              parsedObj.Kind,
					Logger:            hfb.logger,
					HashRegistryCache: nil, // Not needed for fix command execution
					ObjectIDCache:     nil, // Not needed for fix command execution
				}

				// Execute fix command
				success, msg := executeFixCommand(ctx, fixCtx, t.Issue, hfb.storageProvider, hfb.specLoader)

				mu.Lock()
				result.Processed++
				if success {
					result.Fixed++
					logging.Fluent(hfb.logger).Debug("Fix applied").
						String("object_id", t.ObjectID).
						String("message", msg).
						Log()
				} else {
					result.Failed++
					result.Errors = append(result.Errors, fmt.Sprintf("Failed to fix %s: %s", t.ObjectID, t.Issue.Message))
				}
				mu.Unlock()
			})
	}

	// Wait for all fixes to complete with deterministic timeout
	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("hierarchical_fix_wait", "waiting for hierarchical fixes to complete").
		WithCleanup(func() {
			close(waitDone)
		}).
		StartSimple(func() {
			wg.Wait()
		})

	fixTimeout := 5 * time.Minute
	select {
	case <-waitDone:
		// All fixes completed
	case <-time.After(fixTimeout):
		// Timeout - log warning but proceed
		logging.Fluent(hfb.logger).Warn("Timeout waiting for hierarchical fixes to complete").
			String("timeout", fixTimeout.String()).
			Log()
	}

	return result
}

// getKindFromObject extracts the kind from an object map
func getKindFromObject(obj map[string]any) string {
	if kind, ok := obj[objects.FieldKeyKind].(string); ok {
		return kind
	}
	return ""
}
