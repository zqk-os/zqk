package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRuntimeDeltaEnabledForKind_SchedulerJob(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(cfgDir, storage.RuntimeDeltaKindsConfigFileForTest), []byte("kinds:\n  - kind: scheduler_job\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if !storage.RuntimeDeltaEnabledForKind(root, "scheduler_job") {
		t.Fatalf("expected scheduler_job runtime-delta enabled")
	}
	if storage.RuntimeDeltaEnabledForKind(root, "audit_event") {
		t.Fatalf("unexpected runtime-delta enabled for audit_event")
	}
}

func TestRuntimeDeltaEnabledForKind_AgentFeed(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(cfgDir, storage.RuntimeDeltaKindsConfigFileForTest), []byte("kinds:\n  - kind: agent_feed\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if !storage.RuntimeDeltaEnabledForKind(root, "agent_feed") {
		t.Fatalf("expected agent_feed runtime-delta enabled")
	}
}

func TestClassifyUpdateMutation(t *testing.T) {
	tests := []struct {
		name             string
		idUpdated        bool
		runtimeDeltaOnly bool
		want             string
	}{
		{name: "id change wins", idUpdated: true, runtimeDeltaOnly: true, want: storage.UpdateMutationClassIDChangeForTest},
		{name: "runtime delta", idUpdated: false, runtimeDeltaOnly: true, want: storage.UpdateMutationClassRuntimeDeltaForTest},
		{name: "structural", idUpdated: false, runtimeDeltaOnly: false, want: storage.UpdateMutationClassStructuralForTest},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := storage.ClassifyUpdateMutationForTest(tc.idUpdated, tc.runtimeDeltaOnly)
			if got != tc.want {
				t.Fatalf("ClassifyUpdateMutationForTest(%v,%v)=%q want %q", tc.idUpdated, tc.runtimeDeltaOnly, got, tc.want)
			}
		})
	}
}
