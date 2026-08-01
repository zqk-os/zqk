// Tests for stream-backed Read, Exists, and Update (no CAS).
// Exercises readStreamBacked, existsStreamBacked, updateStreamBacked via public API.

package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// streamBackedTestTime is a fixed clock for command_metric fixtures (CreateCleanupTestCommandMetricForTest).
var streamBackedTestTime = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

// enableStreamStorageForTest forces stream-backed paths on for this test. Other tests use
// disableStreamStorageForTest (ZQK_STREAM_STORAGE_ENABLED=0); without this, concurrent runs
// could leave the env at 0 and stream_current would not be written. Uses t.Setenv (no t.Parallel).
func enableStreamStorageForTest(t *testing.T) {
	t.Helper()
	t.Setenv(zqkenv.StreamStorageEnabled(), "1")
}

func setupStreamBackedObjectStorageTest(t *testing.T) (projectRoot string, str storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = fos.Shutdown(context.Background()) }()
	secCtx = pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	str = fos
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})
	return tmpDir, str, secCtx
}

func TestStreamBacked_Read_ReturnsObjectFromStream(t *testing.T) {
	enableStreamStorageForTest(t)
	projectRoot, str, secCtx := setupStreamBackedObjectStorageTest(t)

	storage.BuildPathAliasCacheForProject(projectRoot)
	ctx := pkgctx.NewSystemContext()

	// command_metric is stream-backed (high_volume_kinds.yaml). scheduler_job is CAS + runtime_delta.
	job := storage.CreateCleanupTestCommandMetricForTest("CMD-STREAM-READ", streamBackedTestTime)
	job[objects.FieldKeyTitle] = "Stream read test"
	if err := str.Create(ctx, secCtx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	obj, err := str.Read(ctx, secCtx, "CMD-STREAM-READ")
	if err != nil {
		t.Fatalf("Read (stream-backed): %v", err)
	}
	if obj[objects.FieldKeyKind] != "command_metric" || obj[objects.FieldKeyID] != "CMD-STREAM-READ" {
		t.Errorf("Read: got id=%v kind=%v", obj[objects.FieldKeyID], obj[objects.FieldKeyKind])
	}
}

func TestStreamBacked_Exists_TrueWhenInRegistry_FalseWhenNot(t *testing.T) {
	enableStreamStorageForTest(t)
	projectRoot, str, secCtx := setupStreamBackedObjectStorageTest(t)

	storage.BuildPathAliasCacheForProject(projectRoot)
	ctx := pkgctx.NewSystemContext()

	// Non-existent ID: Exists (stream-backed path) should be false.
	ok, err := str.Exists(ctx, secCtx, "SCH-NONEXISTENT-999")
	if err != nil {
		t.Fatalf("Exists(nonexistent): %v", err)
	}
	if ok {
		t.Error("Exists expected false for nonexistent ID")
	}

	job := storage.CreateCleanupTestCommandMetricForTest("CMD-EXISTS", streamBackedTestTime)
	job[objects.FieldKeyTitle] = "Exists test"
	if err := str.Create(ctx, secCtx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ok, err = str.Exists(ctx, secCtx, "CMD-EXISTS")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !ok {
		t.Error("Exists expected true after Create")
	}
}

func TestStreamBacked_Update_WritesStreamCurrent(t *testing.T) {
	enableStreamStorageForTest(t)
	projectRoot, str, secCtx := setupStreamBackedObjectStorageTest(t)

	storage.BuildPathAliasCacheForProject(projectRoot)
	ctx := pkgctx.NewSystemContext()

	jobID := "CMD-UPDATE-STREAM"
	job := storage.CreateCleanupTestCommandMetricForTest(jobID, streamBackedTestTime)
	job[objects.FieldKeyTitle] = "Before update"
	if err := str.Create(ctx, secCtx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Update (stream-backed path: updateStreamBacked -> WriteStreamBackedCurrentState).
	if err := str.Update(ctx, secCtx, jobID, map[string]any{objects.FieldKeyTitle: "After update", objects.FieldKeyInvocationCount: 42}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// stream_current overlay should exist and contain updated title.
	overlayPath := filepath.Join(datacell.CellStreamOverlayKindDir(projectRoot, "command_metric"), "CMD-UPDATE-STREAM.yaml")
	data, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatalf("read stream_current overlay: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "After update") || !strings.Contains(s, "42") {
		t.Errorf("stream_current should contain updated title and invocation_count; got: %s", s)
	}
	// Read back via API.
	obj, err := str.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("Read after Update: %v", err)
	}
	if obj[objects.FieldKeyTitle] != "After update" {
		t.Errorf("Read title: got %q", obj[objects.FieldKeyTitle])
	}
}

// TestStreamBacked_CreateDuplicate_ReturnsErrObjectExists ensures that creating a stream-backed
// object with an ID that already exists in the stream registry returns ErrObjectExists
// (checkObjectExists uses stream registry only for stream-backed kinds).
func TestStreamBacked_CreateDuplicate_ReturnsErrObjectExists(t *testing.T) {
	enableStreamStorageForTest(t)
	projectRoot, str, secCtx := setupStreamBackedObjectStorageTest(t)

	storage.BuildPathAliasCacheForProject(projectRoot)
	ctx := pkgctx.NewSystemContext()

	jobID := "CMD-DUP-CHECK"
	job := storage.CreateCleanupTestCommandMetricForTest(jobID, streamBackedTestTime)
	job[objects.FieldKeyTitle] = "Duplicate test"
	if err := str.Create(ctx, secCtx, job); err != nil {
		t.Fatalf("Create (first): %v", err)
	}
	// Second Create with same ID must return ErrObjectExists (stream registry already has ID).
	err := str.Create(ctx, secCtx, job)
	if err == nil {
		t.Fatal("expected error when creating duplicate stream-backed command_metric, got nil")
	}
	if err != storage.ErrObjectExists {
		t.Errorf("expected ErrObjectExists, got %v", err)
	}
}
