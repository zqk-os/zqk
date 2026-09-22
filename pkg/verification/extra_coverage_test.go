package verification

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockStorageProvider struct {
	storage.ObjectStorageProvider
	queryResult *storage.QueryResult
	queryErr    error
	countResult int
	countErr    error
}

func (m *mockStorageProvider) Query(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, q storage.Query) (*storage.QueryResult, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return m.queryResult, nil
}

func (m *mockStorageProvider) Count(ctx context.Context, secCtx *storage.SecurityContext, filter storage.ListFilter) (int, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return m.countResult, nil
}

func TestRunVerification_Routing(t *testing.T) {
	ctx := context.Background()

	t.Run("missing strategy", func(t *testing.T) {
		res, err := RunVerification(ctx, nil, nil, map[string]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected failure on missing strategy")
		}
	})

	t.Run("unknown strategy", func(t *testing.T) {
		res, err := RunVerification(ctx, nil, nil, map[string]any{
			"verification_strategy": "unknown_test_xyz",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected failure on unknown strategy")
		}
	})
}

func TestCommandExitCodeStrategy(t *testing.T) {
	ctx := context.Background()
	strat := &CommandExitCodeStrategy{}

	t.Run("missing command", func(t *testing.T) {
		res, err := strat.Verify(ctx, nil, nil, map[string]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected failure for missing command")
		}
	})

	t.Run("successful command with env and artifacts", func(t *testing.T) {
		step := map[string]any{
			objects.FieldKeyCommand: "echo hello",
			"task_id":               "TASK-001",
			objects.FieldKeyArtifacts: []any{
				"file1.txt",
				"file2.txt",
			},
		}
		res, err := strat.Verify(ctx, nil, nil, step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Passed {
			t.Fatalf("expected passed command, feedback: %s", res.Feedback)
		}
	})

	t.Run("failing command", func(t *testing.T) {
		step := map[string]any{
			objects.FieldKeyCommand: "ls /nonexistent_path_definitely_not_here_12345",
		}
		res, err := strat.Verify(ctx, nil, nil, step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected failed command")
		}
	})
}

func TestASTSemanticMatchStrategy(t *testing.T) {
	ctx := context.Background()
	strat := &ASTSemanticMatchStrategy{}

	tempDir := t.TempDir()
	goodFile := filepath.Join(tempDir, "good.go")
	badFile := filepath.Join(tempDir, "bad.go")
	txtFile := filepath.Join(tempDir, "test.txt")

	_ = fileutil.WriteFile(goodFile, []byte("package test\nfunc Good() {}\n"), paths.FilePerm644)
	_ = fileutil.WriteFile(badFile, []byte("package test\nfunc Bad() { panic(\"forbidden\") }\n"), paths.FilePerm644)
	_ = fileutil.WriteFile(txtFile, []byte("forbidden pattern here"), paths.FilePerm644)

	t.Run("forbidden pattern detected in go file", func(t *testing.T) {
		step := map[string]any{
			"config": map[string]any{
				"target_paths":       []any{tempDir},
				"forbidden_patterns": []any{"panic(\"forbidden\")"},
			},
		}
		res, err := strat.Verify(ctx, nil, nil, step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected forbidden pattern to fail")
		}
	})

	t.Run("forbidden pattern not in go file passes", func(t *testing.T) {
		step := map[string]any{
			"config": map[string]any{
				"target_paths":       []any{tempDir},
				"forbidden_patterns": []any{"pattern_that_does_not_exist"},
			},
		}
		res, err := strat.Verify(ctx, nil, nil, step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Passed {
			t.Fatalf("expected pass, got feedback: %s", res.Feedback)
		}
	})
}

func TestQueryMetricStrategy(t *testing.T) {
	ctx := context.Background()
	strat := &QueryMetricStrategy{}

	t.Run("missing config", func(t *testing.T) {
		res, err := strat.Verify(ctx, nil, nil, map[string]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected failure on missing config")
		}
	})

	t.Run("missing query", func(t *testing.T) {
		res, err := strat.Verify(ctx, nil, nil, map[string]any{
			"config": map[string]any{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected failure on missing query")
		}
	})

	t.Run("query error", func(t *testing.T) {
		sp := &mockStorageProvider{queryErr: errors.New("db error")}
		res, err := strat.Verify(ctx, nil, sp, map[string]any{
			"config": map[string]any{
				"query": "MATCH (n) RETURN count(n)",
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected failure on query error")
		}
	})

	t.Run("operators and count fields", func(t *testing.T) {
		cases := []struct {
			name        string
			countVal    any
			operator    string
			threshold   float64
			expectPass  bool
			expectError bool
		}{
			{"gte pass float", 5.0, ">=", 5.0, true, false},
			{"gte fail int", 4, ">=", 5.0, false, false},
			{"lte pass int64", int64(3), "<=", 5.0, true, false},
			{"lte fail float", 6.0, "<=", 5.0, false, false},
			{"eq pass int", 5, "==", 5.0, true, false},
			{"eq fail int", 4, "==", 5.0, false, false},
			{"invalid op", 5, "!=", 5.0, false, false},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				sp := &mockStorageProvider{
					queryResult: &storage.QueryResult{
						Objects: []map[string]any{
							{"count": tc.countVal},
						},
					},
				}
				step := map[string]any{
					"config": map[string]any{
						"query":     "MATCH (n) RETURN count(n)",
						"operator":  tc.operator,
						"threshold": tc.threshold,
					},
				}
				res, err := strat.Verify(ctx, nil, sp, step)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res.Passed != tc.expectPass {
					t.Fatalf("expected pass=%v, got %v (feedback: %s)", tc.expectPass, res.Passed, res.Feedback)
				}
			})
		}
	})

	t.Run("count derived from len(objects)", func(t *testing.T) {
		sp := &mockStorageProvider{
			queryResult: &storage.QueryResult{
				Objects: []map[string]any{
					{"id": "obj1"},
					{"id": "obj2"},
				},
			},
		}
		step := map[string]any{
			"config": map[string]any{
				"query":     "MATCH (n) RETURN n",
				"operator":  ">=",
				"threshold": 2.0,
			},
		}
		res, err := strat.Verify(ctx, nil, sp, step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Passed {
			t.Fatalf("expected pass, got %s", res.Feedback)
		}
	})
}

func TestStateNegationStrategy(t *testing.T) {
	ctx := context.Background()
	strat := &StateNegationStrategy{}

	t.Run("missing config", func(t *testing.T) {
		res, err := strat.Verify(ctx, nil, nil, map[string]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected failure on missing config")
		}
	})

	t.Run("missing target_kind or target_status", func(t *testing.T) {
		res, err := strat.Verify(ctx, nil, nil, map[string]any{
			"config": map[string]any{
				objects.FieldKeyTargetKind: "goal",
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected failure on missing target_status")
		}
	})

	t.Run("storage count error", func(t *testing.T) {
		sp := &mockStorageProvider{countErr: errors.New("count failed")}
		res, err := strat.Verify(ctx, nil, sp, map[string]any{
			"config": map[string]any{
				objects.FieldKeyTargetKind:   "goal",
				objects.FieldKeyTargetStatus: "failed",
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected failure on count error")
		}
	})

	t.Run("negation condition failed when count > 0", func(t *testing.T) {
		sp := &mockStorageProvider{countResult: 2}
		res, err := strat.Verify(ctx, nil, sp, map[string]any{
			"config": map[string]any{
				objects.FieldKeyTargetKind:   "goal",
				objects.FieldKeyTargetStatus: "failed",
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Fatal("expected state negation to fail when count > 0")
		}
	})

	t.Run("negation condition passed when count == 0", func(t *testing.T) {
		sp := &mockStorageProvider{countResult: 0}
		res, err := strat.Verify(ctx, nil, sp, map[string]any{
			"config": map[string]any{
				objects.FieldKeyTargetKind:   "goal",
				objects.FieldKeyTargetStatus: "failed",
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Passed {
			t.Fatal("expected state negation to pass when count == 0")
		}
	})
}
