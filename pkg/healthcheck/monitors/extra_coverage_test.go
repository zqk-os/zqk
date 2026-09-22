// BLI-STARTER-COMMUNITY-058 / PRI-STARTER-COMMUNITY-058 coverage elevation
package monitors

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraMonitorsEmptyAndSynthetic(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	sched := NewSchedulerEventsMonitor()
	if sched.ID() == "" || sched.Name() == "" {
		t.Fatal("scheduler identity")
	}
	if _, err := sched.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := sched.Run(ctx, root); err != nil {
		t.Fatal(err)
	}
	sumPath := scheduler.SummaryPath(root)
	if err := fileutil.MkdirAll(filepath.Dir(sumPath), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(scheduler.SchedulerMetricsSummary{
		WindowEndISO: time.Now().UTC().Format(time.RFC3339),
		JobStats: map[string]scheduler.JobExecutionStats{
			"ok":   {Completed: 2, Failed: 0, TotalDurationSec: 1},
			"fail": {Completed: 1, Failed: 2, TotalDurationSec: 90},
			"slow": {Completed: 1, Failed: 0, TotalDurationSec: 999},
			"zero": {Completed: 0, Failed: 0},
		},
	})
	if err := fileutil.WriteFile(sumPath, body, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := sched.Run(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(sumPath, []byte("{"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	_, _ = sched.Run(ctx, root)
	emptyJobs := t.TempDir()
	ep := scheduler.SummaryPath(emptyJobs)
	_ = fileutil.MkdirAll(filepath.Dir(ep), paths.DirPerm755)
	emptyBody, _ := json.Marshal(scheduler.SchedulerMetricsSummary{JobStats: nil})
	_ = fileutil.WriteFile(ep, emptyBody, paths.FilePerm644)
	_, _ = sched.Run(ctx, emptyJobs)

	ov := &objectVolumeMonitor{}
	_ = ov.ID()
	_ = ov.Name()
	if _, err := ov.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ov.Run(ctx, root); err != nil {
		t.Fatal(err)
	}
	objDir := filepath.Join(root, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsObjectVolumeSubdir)
	if err := fileutil.MkdirAll(objDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if _, err := ov.Run(ctx, root); err != nil {
		t.Fatal(err)
	}
	writeSeries(t, objDir, fmt.Sprintf(objectVolumeSeriesFmt, objects.KindAuditEvent), 100, 2500)
	if _, err := ov.Run(ctx, root); err != nil {
		t.Fatal(err)
	}
	failRoot := t.TempDir()
	failObj := filepath.Join(failRoot, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsObjectVolumeSubdir)
	writeSeries(t, failObj, fmt.Sprintf(objectVolumeSeriesFmt, objects.KindAuditEvent), 100, 9000)
	if _, err := ov.Run(ctx, failRoot); err != nil {
		t.Fatal(err)
	}

	sv := &streamVolumeMonitor{}
	_ = sv.ID()
	_ = sv.Name()
	if _, err := sv.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := sv.Run(ctx, root); err != nil {
		t.Fatal(err)
	}
	streamDir := filepath.Join(root, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsStreamVolumeSubdir)
	if err := fileutil.MkdirAll(streamDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if _, err := sv.Run(ctx, root); err != nil {
		t.Fatal(err)
	}
	writeSeries(t, streamDir, fmt.Sprintf(streamVolumeSeriesFmt, objects.KindAuditEvent), 100, 25000)
	if _, err := sv.Run(ctx, root); err != nil {
		t.Fatal(err)
	}
	failStream := t.TempDir()
	failSD := filepath.Join(failStream, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsStreamVolumeSubdir)
	writeSeries(t, failSD, fmt.Sprintf(streamVolumeSeriesFmt, objects.KindAuditEvent), 100, 90000)
	if _, err := sv.Run(ctx, failStream); err != nil {
		t.Fatal(err)
	}

	wb := &walBacklogMonitor{}
	_ = wb.ID()
	_ = wb.Name()
	if _, err := wb.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.Run(ctx, root); err != nil {
		t.Fatal(err)
	}
	w, err := storage.NewObjectWAL(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(&storage.WALRecord{Op: "create", Kind: "note", ID: "n1"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(&storage.WALRecord{Op: "create", Kind: "note", ID: "n2"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteAppliedSeq(root, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.Run(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteAppliedSeq(root, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.Run(ctx, root); err != nil {
		t.Fatal(err)
	}

	failWAL := t.TempDir()
	walPath := storage.GetWALPath(failWAL)
	if err := fileutil.MkdirAll(filepath.Dir(walPath), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(walPath, []byte(`{"o":"c","k":"note","i":"big","s":30000}`+"\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	_ = storage.WriteAppliedSeq(failWAL, 0)
	if _, err := wb.Run(ctx, failWAL); err != nil {
		t.Fatal(err)
	}
	degradedWAL := t.TempDir()
	dPath := storage.GetWALPath(degradedWAL)
	_ = fileutil.MkdirAll(filepath.Dir(dPath), paths.DirPerm755)
	_ = fileutil.WriteFile(dPath, []byte(`{"o":"c","k":"note","i":"mid","s":6000}`+"\n"), paths.FilePerm644)
	_ = storage.WriteAppliedSeq(degradedWAL, 0)
	if _, err := wb.Run(ctx, degradedWAL); err != nil {
		t.Fatal(err)
	}
	garbageWAL := t.TempDir()
	gPath := storage.GetWALPath(garbageWAL)
	_ = fileutil.MkdirAll(filepath.Dir(gPath), paths.DirPerm755)
	_ = fileutil.WriteFile(gPath, []byte("not-wal\n"), paths.FilePerm644)
	_, _ = wb.Run(ctx, garbageWAL)

	ai := &autonomyInboxMonitor{}
	_ = ai.ID()
	_ = ai.Name()
	if _, err := ai.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	inbox := filepath.Join(root, paths.ProjectDataDir, paths.InboxSubdir)
	if err := fileutil.MkdirAll(inbox, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(inbox, "one.json"), []byte("{}"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	_ = fileutil.MkdirAll(filepath.Join(inbox, "subdir"), paths.DirPerm755)
	if _, err := ai.Run(ctx, root); err != nil {
		t.Fatal(err)
	}
	busy := t.TempDir()
	busyInbox := filepath.Join(busy, paths.ProjectDataDir, paths.InboxSubdir)
	_ = fileutil.MkdirAll(busyInbox, paths.DirPerm755)
	for i := 0; i < 501; i++ {
		_ = fileutil.WriteFile(filepath.Join(busyInbox, fmt.Sprintf("i%d.json", i)), []byte("{}"), paths.FilePerm644)
	}
	if _, err := ai.Run(ctx, busy); err != nil {
		t.Fatal(err)
	}
}

func writeSeries(t *testing.T, dir, series string, v1, v2 int64) {
	t.Helper()
	w, err := metrics.NewTimeSeriesWriter(metrics.TimeSeriesConfig{
		ChunkDuration: objectVolumeChunkWindow,
		Dir:           dir,
		Series:        series,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := w.Append(metrics.TimeSeriesPoint{Ts: now.Add(-2 * time.Hour), Value: v1}); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(metrics.TimeSeriesPoint{Ts: now.Add(-time.Minute), Value: v2}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}
