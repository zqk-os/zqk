package object

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objectidcache"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// applyHybridProjectionToObjectMaps applies hybrid top-level projection to each map (mutates slice elements in place).
func applyHybridProjectionToObjectMaps(objs []map[string]any, fields []string) {
	if len(fields) == 0 || len(objs) == 0 {
		return
	}
	for i := range objs {
		if objs[i] == nil {
			continue
		}
		kind, _ := objs[i][objects.FieldKeyKind].(string)
		mask := objects.HybridMaskForList(kind, fields, "")
		objs[i] = objects.ProjectMapHybrid(objs[i], mask)
	}
}

// applyHybridProjectionToBulkResult applies projection to successful result maps on a BulkResult.
func applyHybridProjectionToBulkResult(result *storagepkg.BulkResult, fields []string) {
	if result == nil || len(fields) == 0 {
		return
	}
	applyHybridProjectionToObjectMaps(result.Results, fields)
}

// setCacheCheckerForBatchCreation sets up the cache checker for batch creation mode
// This enables non-blocking reference validation during batch operations
func setCacheCheckerForBatchCreation(_ *cli.Processor) {
	storagepkg.SetCacheChecker(func(objectID string) (string, bool) {
		cache := objectidcache.GetGlobalObjectIDCache()
		entry, exists := cache.Get(objectID)
		if exists && entry != nil && entry.FilePath != emptyValue {
			return entry.FilePath, true
		}
		return "", false
	})
}

// outputBulkResult outputs bulk operation results using shared utility
func outputBulkResult(cmd *cobra.Command, result *storagepkg.BulkResult, format, operation string) {
	cli.OutputBulkResult(cmd, result, format, operation)
}

func projectAndOutputBulkResult(cmd *cobra.Command, proc *cli.Processor, result *storagepkg.BulkResult, operation string) error {
	projectFields, perr := clipkg.FieldsFromCmd(cmd)
	if perr != nil {
		return cli.Guard(cmd).Err(perr).Return()
	}
	applyHybridProjectionToBulkResult(result, projectFields)
	format := string(proc.Format())
	outputBulkResult(cmd, result, format, operation)
	return nil
}
