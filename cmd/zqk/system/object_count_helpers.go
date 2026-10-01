package system

import (
	"context"

	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck/congruence"
)

// GatherObjectCountByKindForReport returns a single source of truth for object counts by kind,
// aligned with object-count-report and retention targets. Use this for both object-count-report
// and improvement-report so counts are deterministic and comparable.
func GatherObjectCountByKindForReport(ctx context.Context, projectRoot string, storageProvider storage.ObjectStorageProvider, noCache bool) map[string]int {
	return congruence.GatherObjectCountByKindForReport(ctx, projectRoot, storageProvider, noCache)
}

// StreamBackedKindsForReport returns the list of kinds that use stream storage (for report annotations).
func StreamBackedKindsForReport() []string {
	return congruence.StreamBackedKindsForReport()
}

// StreamBackedDirsForReport returns dir names under .zqk/process where object count is from stream storage.
func StreamBackedDirsForReport() []string {
	return congruence.StreamBackedDirsForReport()
}
