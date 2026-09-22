package testjobgen

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	schedcore "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testscan"
)

type mockJobStore struct {
	storage.ObjectStorageProvider
	objs      map[string]map[string]any
	existsMap map[string]bool
}

func newMockJobStore() *mockJobStore {
	return &mockJobStore{
		objs:      make(map[string]map[string]any),
		existsMap: make(map[string]bool),
	}
}

func (m *mockJobStore) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	id, _ := obj[objects.FieldKeyID].(string)
	if m.existsMap[id] {
		return storage.ErrObjectExists
	}
	m.objs[id] = obj
	m.existsMap[id] = true
	return nil
}

func (m *mockJobStore) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objs[id]; ok {
		return obj, nil
	}
	return nil, errors.New("not found")
}

func (m *mockJobStore) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if obj, ok := m.objs[id]; ok {
		for k, v := range updates {
			obj[k] = v
		}
		return nil
	}
	return errors.New("not found")
}

func (m *mockJobStore) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	return m.existsMap[id], nil
}

func (m *mockJobStore) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*storage.BulkResult, error) {
	for _, id := range ids {
		delete(m.objs, id)
		delete(m.existsMap, id)
	}
	return &storage.BulkResult{SuccessCount: len(ids)}, nil
}

func TestHelperFunctions(t *testing.T) {
	t.Run("sanitizeBundleIDForJobID", func(t *testing.T) {
		if got := sanitizeBundleIDForJobID("bundle/pkg#1"); got != "bundle-pkg-1" {
			t.Errorf("got %q, want bundle-pkg-1", got)
		}
		if got := sanitizeBundleIDForJobID("---"); got != "bundle" {
			t.Errorf("got %q, want bundle", got)
		}
	})

	t.Run("hasHeavyAllKindsTests", func(t *testing.T) {
		bLight := &testscan.TestBundle{
			Tests: []*testscan.TestFunction{
				{Name: "TestSimple"},
			},
		}
		if hasHeavyAllKindsTests(bLight) {
			t.Error("expected false for simple bundle")
		}

		bHeavy := &testscan.TestBundle{
			Tests: []*testscan.TestFunction{
				{Name: "TestAllKindsCRUD"},
			},
		}
		if !hasHeavyAllKindsTests(bHeavy) {
			t.Error("expected true for heavy bundle")
		}
	})

	t.Run("priorityForTestBundleJob", func(t *testing.T) {
		if got := priorityForTestBundleJob(nil); got != schedcore.JobPriorityNormal {
			t.Errorf("got %q, want normal", got)
		}
		bNormal := &testscan.TestBundle{}
		if got := priorityForTestBundleJob(bNormal); got != schedcore.JobPriorityNormal {
			t.Errorf("got %q, want normal", got)
		}
		bHigh := &testscan.TestBundle{CriteriaRefs: []string{"CRIT-001"}}
		if got := priorityForTestBundleJob(bHigh); got != schedcore.JobPriorityHigh {
			t.Errorf("got %q, want high", got)
		}
	})

	t.Run("computeTestBundleTimeoutSeconds", func(t *testing.T) {
		bundle := &testscan.TestBundle{
			EstimatedDuration: 10 * time.Second,
			PackagePath:       "pkg/simple",
		}
		sec := computeTestBundleTimeoutSeconds(bundle, "")
		if sec < testBundleMinTimeoutSeconds {
			t.Errorf("expected at least %d, got %d", testBundleMinTimeoutSeconds, sec)
		}
	})

	t.Run("commandStringFromStoredJob", func(t *testing.T) {
		if got := commandStringFromStoredJob(nil); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
		obj := map[string]any{
			objects.FieldKeyCommand:     "go",
			objects.FieldKeyCommandArgs: []any{"test", "./..."},
		}
		cmdStr := commandStringFromStoredJob(obj)
		if !strings.Contains(cmdStr, "go") || !strings.Contains(cmdStr, "./...") {
			t.Errorf("unexpected command string: %s", cmdStr)
		}
	})

	t.Run("contaminationGuardPath", func(t *testing.T) {
		if got := contaminationGuardPath(""); got != "" {
			t.Errorf("expected empty string for empty project root, got %q", got)
		}
	})
}

func TestJobGenerator_GenerateJobs(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockJobStore()
	jg := NewJobGenerator(store, sec)

	tempDir := t.TempDir()

	b1 := &testscan.TestBundle{
		ID:          "b-1",
		PackagePath: "pkg/foo",
		Tests: []*testscan.TestFunction{
			{Name: "TestFoo1"},
		},
		EstimatedDuration: 5 * time.Second,
		IsParallel:        true,
	}
	b2 := &testscan.TestBundle{
		ID:          "b-2",
		PackagePath: "pkg/bar",
		Tests: []*testscan.TestFunction{
			{Name: "TestBar1"},
		},
		EstimatedDuration: 10 * time.Second,
		IsParallel:        false,
	}

	// 1. First run creates jobs
	jobIDs, err := jg.GenerateJobs(ctx, []*testscan.TestBundle{b1, b2}, tempDir, tempDir, 2)
	if err != nil {
		t.Fatalf("GenerateJobs failed: %v", err)
	}
	if len(jobIDs) != 2 {
		t.Fatalf("expected 2 job IDs, got %d", len(jobIDs))
	}

	// 2. Second run reuses active jobs
	jobIDs2, err := jg.GenerateJobs(ctx, []*testscan.TestBundle{b1}, tempDir, tempDir, 2)
	if err != nil {
		t.Fatalf("GenerateJobs second run failed: %v", err)
	}
	if len(jobIDs2) != 1 || jobIDs2[0] != jobIDs[0] {
		t.Fatalf("expected reused job ID %s, got %v", jobIDs[0], jobIDs2)
	}

	// 3. Mark job as archived; next run must recreate it
	store.objs[jobIDs[0]][objects.FieldKeyStatus] = objects.ObjectStatusArchived
	jobIDs3, err := jg.GenerateJobs(ctx, []*testscan.TestBundle{b1}, tempDir, tempDir, 2)
	if err != nil {
		t.Fatalf("GenerateJobs recreate run failed: %v", err)
	}
	if len(jobIDs3) != 1 {
		t.Fatalf("expected 1 job ID, got %d", len(jobIDs3))
	}
	if store.objs[jobIDs3[0]][objects.FieldKeyStatus] != schedcore.StatusActive {
		t.Errorf("expected recreated job status active, got %v", store.objs[jobIDs3[0]][objects.FieldKeyStatus])
	}
}

func TestBuildDescription(t *testing.T) {
	jg := &JobGenerator{}
	bundle := &testscan.TestBundle{
		PackagePath:       "pkg/sample",
		Tests:             []*testscan.TestFunction{{Name: "TestA"}},
		EstimatedDuration: 2 * time.Second,
		IsParallel:        true,
	}
	desc := jg.buildDescription(bundle, "/path/to/log.log")
	if !strings.Contains(desc, "pkg/sample") || !strings.Contains(desc, "TestA") {
		t.Errorf("unexpected description: %s", desc)
	}
}

func TestScanAndSchedule_NoTests(t *testing.T) {
	tempDir := t.TempDir()
	store := newMockJobStore()
	sec := pkgctx.NewSystemSecurityContext()
	_, err := ScanAndSchedule(context.Background(), tempDir, store, sec, 10, 2)
	if err == nil {
		t.Fatal("expected error when no tests found")
	}
}

