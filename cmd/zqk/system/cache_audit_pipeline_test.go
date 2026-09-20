package system

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestDetectStaleEntriesFromByKind_SingleMissingFile(t *testing.T) {
	tmp := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmp)
	kind := "policy"
	kindDir := objects.GetDirectoryFromKind(kind)

	fixedMTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	existPath := ""
	if kindDir != emptyValue {
		existPath = filepath.Join(processDir, kindDir, "exist.yaml")
	} else {
		existPath = filepath.Join(processDir, "exist.yaml")
	}
	if err := fileutil.EnsureDir(filepath.Dir(existPath)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := fileutil.WriteSecureFile(existPath, []byte("test")); err != nil {
		t.Fatalf("write exist file: %v", err)
	}

	v2 := ObjectIDCacheFileV2{
		Metadata: &ObjectIDCacheMetadata{
			BuildTime:    time.Now(),
			ProjectRoot:  tmp,
			ProcessDir:   processDir,
			ProcessMTime: fixedMTime,
		},
		CountByKind: map[string]int{kind: 2},
		ByKind: map[string][]KindBucketEntry{
			kind: {
				{ID: "EXIST", Path: "exist.yaml", MTime: fixedMTime},
				{ID: "MISSING", Path: "missing.yaml", MTime: fixedMTime},
			},
		},
	}

	data, err := json.Marshal(v2)
	if err != nil {
		t.Fatalf("marshal cache json: %v", err)
	}

	byKind, _, metadata, _, err := ParseObjectIDCacheFile(data)
	if err != nil {
		t.Fatalf("ParseObjectIDCacheFile: %v", err)
	}
	if metadata == nil {
		t.Fatalf("metadata nil")
	}

	stale := detectStaleEntriesFromByKind(byKind, tmp, datacell.ProcessPrimaryDir(metadata.ProjectRoot), fileutil.Stat,
		func(projectRoot, filePath string) *GitDeletionInfo { return nil })

	if len(stale) != 1 {
		t.Fatalf("expected 1 stale entry, got %d", len(stale))
	}
	if stale[0].ID != "MISSING" || stale[0].Kind != kind {
		t.Fatalf("unexpected stale entry: %+v", stale[0])
	}
	if stale[0].CachedMTime != fixedMTime {
		t.Fatalf("unexpected CachedMTime: %v want %v", stale[0].CachedMTime, fixedMTime)
	}

	wantMissingPath := ""
	if kindDir != emptyValue {
		wantMissingPath = filepath.Join(processDir, kindDir, "missing.yaml")
	} else {
		wantMissingPath = filepath.Join(processDir, "missing.yaml")
	}
	if stale[0].FilePath != wantMissingPath {
		t.Fatalf("unexpected FilePath: %v want %v", stale[0].FilePath, wantMissingPath)
	}
}
