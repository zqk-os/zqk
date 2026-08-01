package storage_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestStreamRegistry_ListGetDeleteCountAcrossProcesses ensures that when stream-backed
// objects are created in one "process" (storage instance), a second process (new storage
// instance with same projectRoot, no in-memory stream state) can List, Get, Delete, and
// see Count decrease via the persistent registry and deleted set. This locks in the fix
// for retention/CLI being able to list and delete stream-backed audit_events so counts can drop.
func TestStreamRegistry_ListGetDeleteCountAcrossProcesses(t *testing.T) {

	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	projectRoot := tmpDir
	var storage2 *storage.FileObjectStorage
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if storage2 != nil {
			_ = storage2.Shutdown(ctx)
		}
	})

	storage.BuildPathAliasCacheForProject(projectRoot) // required for AppendToStream (stream segment dir resolution)
	createdAt, _ := time.Parse(time.RFC3339, "2030-03-01T12:00:00Z")

	// "Process A": write two audit_events to stream and register them (simulates Create in daemon).
	obj1 := map[string]any{
		objects.FieldKeyID: "AUD-REG-1", objects.FieldKeyKind: "audit_event",
		objects.FieldKeyEventType: "object_creation", objects.FieldKeyCreatedAt: createdAt.Format(time.RFC3339),
	}
	obj2 := map[string]any{
		objects.FieldKeyID: "AUD-REG-2", objects.FieldKeyKind: "audit_event",
		objects.FieldKeyEventType: "object_creation", objects.FieldKeyCreatedAt: createdAt.Format(time.RFC3339),
	}

	seg1, off1, err := storage.AppendToStream(projectRoot, "audit_event", "AUD-REG-1", obj1, createdAt)
	if err != nil {
		t.Fatalf("AppendToStream 1: %v", err)
	}
	if err := storage.AppendStreamLocationToRegistry(projectRoot, "audit_event", "AUD-REG-1", seg1+"::"+strconv.FormatInt(off1, 10)); err != nil {
		t.Fatalf("AppendStreamLocationToRegistry 1: %v", err)
	}

	seg2, off2, err := storage.AppendToStream(projectRoot, "audit_event", "AUD-REG-2", obj2, createdAt)
	if err != nil {
		t.Fatalf("AppendToStream 2: %v", err)
	}
	if err := storage.AppendStreamLocationToRegistry(projectRoot, "audit_event", "AUD-REG-2", seg2+"::"+strconv.FormatInt(off2, 10)); err != nil {
		t.Fatalf("AppendStreamLocationToRegistry 2: %v", err)
	}

	// "Process B": new storage instance (e.g. CLI or second daemon) — no in-memory stream locations.
	storage2, err = storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorage (second process): %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	// List must see both IDs from persistent registry.
	filter := storage.ListFilter{Kind: "audit_event"}
	result, err := storage2.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Objects) != 2 {
		t.Fatalf("List: got %d objects, want 2", len(result.Objects))
	}
	ids := make(map[string]bool)
	for _, o := range result.Objects {
		if id, _ := o[objects.FieldKeyID].(string); id != "" {
			ids[id] = true
		}
	}
	if !ids["AUD-REG-1"] || !ids["AUD-REG-2"] {
		t.Errorf("List: missing IDs, got %v", ids)
	}

	// Get must resolve via persistent registry.
	got1, err := storage2.Read(ctx, secCtx, "AUD-REG-1")
	if err != nil {
		t.Fatalf("Read AUD-REG-1: %v", err)
	}
	if got1[objects.FieldKeyID] != "AUD-REG-1" || got1[objects.FieldKeyKind] != "audit_event" {
		t.Errorf("Read AUD-REG-1: got %v", got1)
	}

	// Count must include stream-backed (segment line count minus deleted).
	countBefore, err := storage2.Count(ctx, secCtx, filter)
	if err != nil {
		t.Fatalf("Count before delete: %v", err)
	}
	if countBefore < 2 {
		t.Errorf("Count before delete: got %d, want at least 2", countBefore)
	}

	// Delete one (soft-delete for stream: add to deleted set).
	cliCtx := storage.WithCLIOperation(ctx)
	if err := storage2.Delete(cliCtx, secCtx, "AUD-REG-1", false); err != nil {
		t.Fatalf("Delete AUD-REG-1: %v", err)
	}

	// List must now see only AUD-REG-2 (deleted IDs excluded).
	result2, err := storage2.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(result2.Objects) != 1 {
		t.Fatalf("List after delete: got %d objects, want 1", len(result2.Objects))
	}
	if result2.Objects[0][objects.FieldKeyID] != "AUD-REG-2" {
		t.Errorf("List after delete: got %v", result2.Objects[0][objects.FieldKeyID])
	}

	// Count must decrease (deleted set is subtracted).
	countAfter, err := storage2.Count(ctx, secCtx, filter)
	if err != nil {
		t.Fatalf("Count after delete: %v", err)
	}
	if countAfter != countBefore-1 {
		t.Errorf("Count after delete: got %d, want %d (before - 1)", countAfter, countBefore-1)
	}

	// Get for deleted ID must not succeed (registry hides deleted IDs; may return ErrObjectNotFound or path resolution error).
	_, err = storage2.Read(ctx, secCtx, "AUD-REG-1")
	if err == nil {
		t.Error("Read AUD-REG-1 after delete: expected error (object should not be visible)")
	}
}

// TestCompactStreamRegistryForKind ensures compaction rewrites the registry to live-only IDs
// and truncates the deleted file so state does not grow indefinitely.
func TestCompactStreamRegistryForKind(t *testing.T) {

	dir := t.TempDir()
	stateDir := filepath.Join(dir, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.EnsureDir(stateDir); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	kind := "audit_event"
	delPath := filepath.Join(stateDir, "stream_deleted_"+kind+".jsonl")

	// "id1" hashes to shard 14 (105+100+49 = 254 % 16 = 14)
	// "id2" hashes to shard 15 (105+100+50 = 255 % 16 = 15)
	shardPath1 := fmt.Sprintf("%s_%s_%02d.jsonl", filepath.Join(stateDir, "stream_registry"), kind, 14)
	shardPath2 := fmt.Sprintf("%s_%s_%02d.jsonl", filepath.Join(stateDir, "stream_registry"), kind, 15)

	if err := fileutil.WriteSecureFile(shardPath1, []byte(`{"`+objects.FieldKeyID+`":"id1","loc":"seg1::0"}`+"\n")); err != nil {
		t.Fatalf("write shard 14: %v", err)
	}
	if err := fileutil.WriteSecureFile(shardPath2, []byte(`{"`+objects.FieldKeyID+`":"id2","loc":"seg1::1"}`+"\n")); err != nil {
		t.Fatalf("write shard 15: %v", err)
	}
	if err := fileutil.WriteSecureFile(delPath, []byte("id1\n")); err != nil {
		t.Fatalf("write deleted: %v", err)
	}

	if err := storage.CompactStreamRegistryForKind(dir, kind); err != nil {
		t.Fatalf("CompactStreamRegistryForKind: %v", err)
	}

	// Shard 15 should contain only id2.
	var regLines []string
	f, err := os.Open(shardPath2)
	if err != nil {
		t.Fatalf("open registry shard 15: %v", err)
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var row map[string]string
		if json.Unmarshal([]byte(line), &row) == nil {
			regLines = append(regLines, row[objects.FieldKeyID])
		}
	}
	_ = f.Close()
	if len(regLines) != 1 || regLines[0] != "id2" {
		t.Errorf("registry after compact: got %v, want [id2]", regLines)
	}

	// Deleted file should be empty (truncated).
	b, err := os.ReadFile(delPath)
	if err != nil {
		t.Fatalf("read deleted: %v", err)
	}
	if len(strings.TrimSpace(string(b))) != 0 {
		t.Errorf("deleted file after compact: got %q, want empty", b)
	}
}

func TestCompactStreamRegistryForKind_MergesDailySegments(t *testing.T) {
	dir := t.TempDir()
	storage.BuildPathAliasCacheForProject(dir) // required to resolve streams segment dir

	stateDir := filepath.Join(dir, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.EnsureDir(stateDir); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	kind := "audit_event"
	streamDir := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir, kind)
	if err := fileutil.EnsureDir(streamDir); err != nil {
		t.Fatalf("MkdirAll streamDir: %v", err)
	}

	// Create daily process segment files for a past date "2020-01-01"
	file1 := filepath.Join(streamDir, "2020-01-01_pid123_stream.json")
	file2 := filepath.Join(streamDir, "2020-01-01_pid456_stream.json")

	rec1 := fmt.Sprintf(`{"%s":"evt1","%s":"audit_event","message":"hello"}`+"\n", objects.FieldKeyID, objects.FieldKeyKind)
	rec2 := fmt.Sprintf(`{"%s":"evt2","%s":"audit_event","message":"world"}`+"\n", objects.FieldKeyID, objects.FieldKeyKind)
	rec3 := fmt.Sprintf(`{"%s":"evt3","%s":"audit_event","message":"!!!"}`+"\n", objects.FieldKeyID, objects.FieldKeyKind)

	if err := fileutil.WriteSecureFile(file1, []byte(rec1+rec2)); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := fileutil.WriteSecureFile(file2, []byte(rec3)); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	// Populate stream registry shards.
	// "evt1" hashes to shard 0 (101+118+116+49 = 384 % 16 = 0)
	// "evt2" hashes to shard 1 (101+118+116+50 = 385 % 16 = 1)
	// "evt3" hashes to shard 2 (101+118+116+51 = 386 % 16 = 2)
	shardPath0 := fmt.Sprintf("%s_%s_%02d.jsonl", filepath.Join(stateDir, "stream_registry"), kind, 0)
	shardPath1 := fmt.Sprintf("%s_%s_%02d.jsonl", filepath.Join(stateDir, "stream_registry"), kind, 1)
	shardPath2 := fmt.Sprintf("%s_%s_%02d.jsonl", filepath.Join(stateDir, "stream_registry"), kind, 2)

	// Format loc using the original filenames
	loc1 := fmt.Sprintf("%s::%d", file1, 0)
	loc2 := fmt.Sprintf("%s::%d", file1, len(rec1))
	loc3 := fmt.Sprintf("%s::%d", file2, 0)

	if err := fileutil.WriteSecureFile(shardPath0, []byte(fmt.Sprintf(`{"%s":"evt1","loc":"%s"}`+"\n", objects.FieldKeyID, loc1))); err != nil {
		t.Fatalf("write shard 0: %v", err)
	}
	if err := fileutil.WriteSecureFile(shardPath1, []byte(fmt.Sprintf(`{"%s":"evt2","loc":"%s"}`+"\n", objects.FieldKeyID, loc2))); err != nil {
		t.Fatalf("write shard 1: %v", err)
	}
	if err := fileutil.WriteSecureFile(shardPath2, []byte(fmt.Sprintf(`{"%s":"evt3","loc":"%s"}`+"\n", objects.FieldKeyID, loc3))); err != nil {
		t.Fatalf("write shard 2: %v", err)
	}

	// Soft-delete "evt2"
	delPath := filepath.Join(stateDir, "stream_deleted_"+kind+".jsonl")
	if err := fileutil.WriteSecureFile(delPath, []byte("evt2\n")); err != nil {
		t.Fatalf("write deleted: %v", err)
	}

	// Run compaction
	if err := storage.CompactStreamRegistryForKind(dir, kind); err != nil {
		t.Fatalf("CompactStreamRegistryForKind: %v", err)
	}

	// Verify old files are gone
	if _, err := os.Stat(file1); err == nil || !os.IsNotExist(err) {
		t.Error("file1 should be deleted")
	}
	if _, err := os.Stat(file2); err == nil || !os.IsNotExist(err) {
		t.Error("file2 should be deleted")
	}

	// Verify unified file exists
	unifiedPath := filepath.Join(streamDir, "2020-01-01_stream.json")
	if _, err := os.Stat(unifiedPath); err != nil {
		t.Fatalf("unified file should exist: %v", err)
	}

	// Unified file should contain evt1 and evt3, but not evt2
	b, err := os.ReadFile(unifiedPath)
	if err != nil {
		t.Fatalf("read unified file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("unified file should have exactly 2 lines, got %d:\n%s", len(lines), string(b))
	}
	expectedID1 := fmt.Sprintf(`"%s":"evt1"`, objects.FieldKeyID)
	if !strings.Contains(lines[0], expectedID1) {
		t.Errorf("line 0 should be evt1, got %q", lines[0])
	}
	expectedID3 := fmt.Sprintf(`"%s":"evt3"`, objects.FieldKeyID)
	if !strings.Contains(lines[1], expectedID3) {
		t.Errorf("line 1 should be evt3, got %q", lines[1])
	}

	// Registry shard 0 (evt1) and shard 2 (evt3) should point to unifiedPath with correct new offsets
	b0, err := os.ReadFile(shardPath0)
	if err != nil {
		t.Fatalf("read shard 0: %v", err)
	}
	var row0 map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(b0), &row0); err != nil {
		t.Fatalf("unmarshal shard 0: %v", err)
	}
	expectedLoc0 := fmt.Sprintf("%s::%d", unifiedPath, 0)
	if row0["loc"] != expectedLoc0 {
		t.Errorf("shard 0 loc: got %q, want %q", row0["loc"], expectedLoc0)
	}

	b2, err := os.ReadFile(shardPath2)
	if err != nil {
		t.Fatalf("read shard 2: %v", err)
	}
	var row2 map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(b2), &row2); err != nil {
		t.Fatalf("unmarshal shard 2: %v", err)
	}
	expectedLoc2 := fmt.Sprintf("%s::%d", unifiedPath, len(lines[0])+1) // evt1 line plus newline character
	if row2["loc"] != expectedLoc2 {
		t.Errorf("shard 2 loc: got %q, want %q", row2["loc"], expectedLoc2)
	}

	// Shard 1 (evt2) should be empty
	b1, err := os.ReadFile(shardPath1)
	if err != nil {
		t.Fatalf("read shard 1: %v", err)
	}
	if len(strings.TrimSpace(string(b1))) != 0 {
		t.Errorf("shard 1 should be empty, got %q", string(b1))
	}
}
