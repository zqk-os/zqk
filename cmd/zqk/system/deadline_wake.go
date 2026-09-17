package system

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ScheduleDeadlineWakeSignal writes a lightweight timer directly into the hourglass
// directory so the scheduler's background watcher can wake up natively in Go.
func ScheduleDeadlineWakeSignal(ctx context.Context, projectRoot string, kind, id, deadlineStr string) {
	if deadlineStr == "" || projectRoot == "" {
		return
	}

	var nextRunAt string
	if len(deadlineStr) == 10 { // YYYY-MM-DD
		nextRunAt = fmt.Sprintf("%sT00:00:00Z", deadlineStr)
	} else {
		_, err := time.Parse(time.RFC3339, deadlineStr)
		if err == nil {
			nextRunAt = deadlineStr
		} else {
			return
		}
	}

	schedulerRoot := paths.ResolvePathFromCacheOrConstant(projectRoot, "scheduler", filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
	hourglassDir := filepath.Join(schedulerRoot, "hourglass")
	_ = fileutil.MkdirAll(hourglassDir, 0755)

	filePath := filepath.Join(hourglassDir, id+".json")

	info := map[string]any{
		"task_id":                 id,
		objects.FieldKeyKind:      kind,
		objects.FieldKeyType:      "deadline",
		objects.FieldKeyExpiresAt: nextRunAt,
	}

	data, err := json.Marshal(info)
	if err == nil {
		_ = fileutil.WriteFile(filePath, data, 0644)
	}
}
