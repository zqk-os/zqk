package organizational

import (
	"context"
	"errors"
	"io"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type memoryMockStorage struct {
	storage.ObjectStorageProvider
	objs      map[string]map[string]any
	readErr   error
	createErr error
	updateErr error
	listErr   error
}

func newMemoryMockStorage() *memoryMockStorage {
	return &memoryMockStorage{
		objs: make(map[string]map[string]any),
	}
}

func (m *memoryMockStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if m.createErr != nil {
		return m.createErr
	}
	id, _ := obj[objects.FieldKeyID].(string)
	if id == "" {
		id = "GEN-12345"
		obj[objects.FieldKeyID] = id
	}
	m.objs[id] = obj
	return nil
}

func (m *memoryMockStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	obj, ok := m.objs[id]
	if !ok {
		return nil, errors.New("object not found")
	}
	return obj, nil
}

func (m *memoryMockStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.objs[id] = updates
	return nil
}

func (m *memoryMockStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []map[string]any
	for _, obj := range m.objs {
		if filter.Kind == "" || obj[objects.FieldKeyKind] == filter.Kind {
			res = append(res, obj)
		}
	}
	return &storage.QueryResult{Objects: res}, nil
}

func TestImpactAnalyzer_AnalyzeChange(t *testing.T) {
	secCtx := &pkgctx.SecurityContext{}
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))

	t.Run("read error", func(t *testing.T) {
		m := newMemoryMockStorage()
		m.readErr = errors.New("disk failure")
		analyzer := NewImpactAnalyzer(m, logger, secCtx)
		_, err := analyzer.AnalyzeChange(context.Background(), "OCH-001")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("wrong kind error", func(t *testing.T) {
		m := newMemoryMockStorage()
		m.objs["GOAL-001"] = map[string]any{
			objects.FieldKeyKind: objects.KindGoal,
			objects.FieldKeyID:   "GOAL-001",
		}
		analyzer := NewImpactAnalyzer(m, logger, secCtx)
		_, err := analyzer.AnalyzeChange(context.Background(), "GOAL-001")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("create error", func(t *testing.T) {
		m := newMemoryMockStorage()
		m.objs["OCH-001"] = map[string]any{
			objects.FieldKeyKind:       objects.KindOrganizationalChange,
			objects.FieldKeyID:         "OCH-001",
			objects.FieldKeyChangeType: "reorg",
			objects.FieldKeyAffectedObjects: map[string]any{
				"divisions": []any{"DIV-1"},
			},
		}
		m.createErr = errors.New("cannot create impact analysis")
		analyzer := NewImpactAnalyzer(m, logger, secCtx)
		_, err := analyzer.AnalyzeChange(context.Background(), "OCH-001")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("successful analysis and reference update", func(t *testing.T) {
		m := newMemoryMockStorage()
		m.objs["OCH-001"] = map[string]any{
			objects.FieldKeyKind:       objects.KindOrganizationalChange,
			objects.FieldKeyID:         "OCH-001",
			objects.FieldKeyChangeType: "restructure",
			objects.FieldKeyAffectedObjects: map[string]any{
				"divisions": []any{"DIV-1"},
				"teams":     []any{"domain:organizational:team:TEAM-A"},
			},
		}
		m.objs["WS-1"] = map[string]any{
			objects.FieldKeyKind:        objects.KindWorkstream,
			objects.FieldKeyID:          "WS-1",
			objects.FieldKeyDivisionRef: "DIV-1",
		}
		m.objs["GOAL-1"] = map[string]any{
			objects.FieldKeyKind: objects.KindGoal,
			objects.FieldKeyID:   "GOAL-1",
			"team_ref":           []any{"TEAM-A"},
		}
		m.objs["BLI-1"] = map[string]any{
			objects.FieldKeyKind: objects.KindBacklogItem,
			objects.FieldKeyID:   "BLI-1",
			"team_ref":           "domain:organizational:team:TEAM-A",
		}
		m.objs["MIL-1"] = map[string]any{
			objects.FieldKeyKind:        objects.KindMilestone,
			objects.FieldKeyID:          "MIL-1",
			objects.FieldKeyDivisionRef: "DIV-OTHER",
		}

		analyzer := NewImpactAnalyzer(m, logger, secCtx)
		id, err := analyzer.AnalyzeChange(context.Background(), "OCH-001")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id == "" {
			t.Fatal("expected non-empty impact analysis id")
		}

		// Verify OCH-001 was updated with impact analysis ref
		och := m.objs["OCH-001"]
		refs, ok := och[objects.FieldKeyImpactAnalysisRefs].([]any)
		if !ok || len(refs) == 0 {
			t.Fatalf("expected impact analysis refs in OCH-001, got %v", och[objects.FieldKeyImpactAnalysisRefs])
		}
		if refs[0] != id {
			t.Fatalf("expected ref %q, got %v", id, refs[0])
		}
	})

	t.Run("update error does not fail analysis", func(t *testing.T) {
		m := newMemoryMockStorage()
		m.objs["OCH-002"] = map[string]any{
			objects.FieldKeyKind:       objects.KindOrganizationalChange,
			objects.FieldKeyID:         "OCH-002",
			objects.FieldKeyChangeType: "restructure",
		}
		m.updateErr = errors.New("update lock conflict")
		analyzer := NewImpactAnalyzer(m, logger, secCtx)
		id, err := analyzer.AnalyzeChange(context.Background(), "OCH-002")
		if err != nil {
			t.Fatalf("expected success despite update error, got: %v", err)
		}
		if id == "" {
			t.Fatal("expected non-empty impact analysis id")
		}
	})
}

func TestReferenceTraversal_HelperFunctions(t *testing.T) {
	t.Run("extractOrgObjectIDs", func(t *testing.T) {
		affected := map[string]any{
			"divisions": []any{"DIV-1", "", 123},
			"teams":     []any{"TEAM-1", "TEAM-2"},
			"invalid":   "not a slice",
		}
		ids := extractOrgObjectIDs(affected)
		if len(ids) != 3 {
			t.Fatalf("expected 3 valid IDs, got %d (%v)", len(ids), ids)
		}
	})

	t.Run("extractNamespaceID", func(t *testing.T) {
		if got := extractNamespaceID("plain-id"); got != "plain-id" {
			t.Errorf("got %q, want plain-id", got)
		}
		if got := extractNamespaceID("ns:part:id"); got != "ns:part:id" {
			t.Errorf("got %q, want ns:part:id", got)
		}
	})

	t.Run("extractIDFromNamespace", func(t *testing.T) {
		if got := extractIDFromNamespace("domain:org:DIV-001"); got != "DIV-001" {
			t.Errorf("got %q, want DIV-001", got)
		}
		if got := extractIDFromNamespace("DIV-002"); got != "DIV-002" {
			t.Errorf("got %q, want DIV-002", got)
		}
	})

	t.Run("getOrgReferenceFields", func(t *testing.T) {
		for _, k := range []string{objects.KindWorkstream, objects.KindGoal, objects.KindBacklogItem, objects.KindMilestone} {
			fields := getOrgReferenceFields(k)
			if len(fields) == 0 {
				t.Errorf("expected reference fields for %s", k)
			}
		}
		if fields := getOrgReferenceFields("unknown_kind"); len(fields) != 0 {
			t.Errorf("expected empty fields for unknown kind, got %v", fields)
		}
	})

	t.Run("findObjectsReferencingOrgObjects empty list", func(t *testing.T) {
		m := newMemoryMockStorage()
		res, err := findObjectsReferencingOrgObjects(context.Background(), m, objects.KindGoal, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 0 {
			t.Fatalf("expected 0 results, got %d", len(res))
		}
	})

	t.Run("findObjectsReferencingOrgObjects list error", func(t *testing.T) {
		m := newMemoryMockStorage()
		m.listErr = errors.New("list failed")
		_, err := findObjectsReferencingOrgObjects(context.Background(), m, objects.KindGoal, []string{"DIV-1"}, nil, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("referencesAnyOrgObject matches single and slice", func(t *testing.T) {
		orgSet := map[string]bool{"DIV-1": true, "TEAM-1": true}
		refFields := []string{"div_ref", "team_refs"}

		obj1 := map[string]any{"div_ref": "DIV-1"}
		if !referencesAnyOrgObject(obj1, orgSet, refFields) {
			t.Error("expected match for obj1")
		}

		obj2 := map[string]any{"team_refs": []any{"domain:org:TEAM-1"}}
		if !referencesAnyOrgObject(obj2, orgSet, refFields) {
			t.Error("expected match for obj2")
		}

		obj3 := map[string]any{"div_ref": "DIV-999"}
		if referencesAnyOrgObject(obj3, orgSet, refFields) {
			t.Error("expected no match for obj3")
		}
	})
}
