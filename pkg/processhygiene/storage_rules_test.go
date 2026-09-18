package processhygiene

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var errStubStorage = errors.New("stub storage")

// stubObjectStorage implements [storage.ObjectStorageProvider] with a configurable List for tests.
type stubObjectStorage struct {
	listObjs []map[string]any
}

func (s *stubObjectStorage) List(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if filter.Kind != kindProcessHygieneRule {
		return &storage.QueryResult{}, nil
	}
	return &storage.QueryResult{Objects: s.listObjs}, nil
}

func (*stubObjectStorage) Create(context.Context, *storage.SecurityContext, map[string]any) error {
	return errStubStorage
}

func (*stubObjectStorage) GenerateID(ctx context.Context, kind string) (string, error) {
	return "MOCK-123", nil
}
func (*stubObjectStorage) Read(context.Context, *storage.SecurityContext, string) (map[string]any, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) Update(context.Context, *storage.SecurityContext, string, map[string]any) error {
	return errStubStorage
}
func (*stubObjectStorage) Delete(context.Context, *storage.SecurityContext, string, bool) error {
	return errStubStorage
}
func (*stubObjectStorage) Query(context.Context, *storage.SecurityContext, *pkgctx.StorageContext, storage.Query) (*storage.QueryResult, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) Search(context.Context, *storage.SecurityContext, *pkgctx.StorageContext, storage.SearchQuery) (*storage.SearchResult, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) BeginTransaction(context.Context) (storage.ObjectTransaction, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) BulkCreate(context.Context, *storage.SecurityContext, []map[string]any) (*storage.BulkResult, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) BulkUpdate(context.Context, *storage.SecurityContext, []storage.BulkUpdateItem) (*storage.BulkResult, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) BulkGet(context.Context, *storage.SecurityContext, []string) (*storage.BulkResult, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) BulkDelete(context.Context, *storage.SecurityContext, []string, bool) (*storage.BulkResult, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) Exists(context.Context, *storage.SecurityContext, string) (bool, error) {
	return false, errStubStorage
}
func (*stubObjectStorage) Count(context.Context, *storage.SecurityContext, storage.ListFilter) (int, error) {
	return 0, errStubStorage
}
func (s *stubObjectStorage) Aggregate(context.Context, *storage.SecurityContext, *pkgctx.StorageContext, storage.ListFilter, []storage.Aggregation) (*storage.AggregateResult, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) GetRelated(context.Context, *storage.SecurityContext, string, string, int) ([]map[string]any, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) GetPath(context.Context, *storage.SecurityContext, string, string) ([]map[string]any, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) GetNeighbors(context.Context, *storage.SecurityContext, string, string) ([]map[string]any, error) {
	return nil, errStubStorage
}
func (*stubObjectStorage) Move(context.Context, *storage.SecurityContext, string, string, bool) error {
	return errStubStorage
}
func (*stubObjectStorage) Rename(context.Context, *storage.SecurityContext, string, string, bool) error {
	return errStubStorage
}

func TestLoadRulesFromStorage_SkipDisabledAndSortOrder(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	st := &stubObjectStorage{listObjs: []map[string]any{
		{
			objects.FieldKeyRuleID: "z_second", objects.FieldKeyMatchField: objects.FieldKeyTitle,
			objects.FieldKeyMatchPrefix: "Z", objects.FieldKeySortOrder: 2, objects.FieldKeyEnabled: true,
		},
		{
			objects.FieldKeyRuleID: "a_first", objects.FieldKeyMatchField: objects.FieldKeyTitle,
			objects.FieldKeyMatchPrefix: "A", objects.FieldKeySortOrder: 1, objects.FieldKeyEnabled: true,
		},
		{
			objects.FieldKeyRuleID: "disabled", objects.FieldKeyMatchField: objects.FieldKeyTitle,
			objects.FieldKeyMatchPrefix: "D", objects.FieldKeyEnabled: false,
		},
	}}
	rules, err := loadRulesFromStorage(ctx, st, sec)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("want 2 rules, got %d", len(rules))
	}
	if rules[0].ID() != "a_first" || rules[1].ID() != "z_second" {
		t.Fatalf("order: got %s then %s", rules[0].ID(), rules[1].ID())
	}
}

func TestResolveRulesWithOptions_StorageNonEmpty(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	st := &stubObjectStorage{listObjs: []map[string]any{
		{
			objects.FieldKeyRuleID: "stor_only", objects.FieldKeyMatchField: objects.FieldKeyTitle,
			objects.FieldKeyMatchEquals: "hello", objects.FieldKeyEnabled: true,
		},
	}}
	rules, src, err := ResolveRulesWithOptions(ResolveRulesOptions{
		ProjectRoot:     t.TempDir(),
		RulesFile:       "",
		StorageProvider: st,
		SecCtx:          sec,
		ListCtx:         ctx,
	})
	if err != nil {
		t.Fatal(err)
	}
	if src != "storage:process_hygiene_rule" {
		t.Fatalf("src: %q", src)
	}
	if len(rules) != 1 || rules[0].ID() != "stor_only" {
		t.Fatalf("rules: %+v", rules)
	}
}

func TestResolveRulesWithOptions_StorageEmptyFallsBackToEmbedded(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	st := &stubObjectStorage{listObjs: nil}
	rules, src, err := ResolveRulesWithOptions(ResolveRulesOptions{
		ProjectRoot:     t.TempDir(),
		RulesFile:       "",
		StorageProvider: st,
		SecCtx:          sec,
		ListCtx:         ctx,
	})
	if err != nil {
		t.Fatal(err)
	}
	if src != "embedded:default_rules.yaml" {
		t.Fatalf("want embedded fallback, got src %q", src)
	}
	if len(rules) < 1 {
		t.Fatalf("expected embedded rules")
	}
}

func TestResolveRulesWithOptions_RulesFileOverrides(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	data := `rules:
  - id: file_rule
    description: x
    match:
      field: title
      equals: "only-from-file"
`
	if err := fileutil.WriteSecureFile(path, []byte(data)); err != nil {
		t.Fatal(err)
	}
	st := &stubObjectStorage{listObjs: []map[string]any{
		{
			objects.FieldKeyRuleID: "from_storage", objects.FieldKeyMatchField: objects.FieldKeyTitle,
			objects.FieldKeyMatchEquals: "x", objects.FieldKeyEnabled: true,
		},
	}}
	rules, src, err := ResolveRulesWithOptions(ResolveRulesOptions{
		ProjectRoot:     dir,
		RulesFile:       path,
		StorageProvider: st,
		SecCtx:          sec,
		ListCtx:         ctx,
	})
	if err != nil {
		t.Fatal(err)
	}
	if src != path {
		t.Fatalf("src: %q", src)
	}
	if len(rules) != 1 || rules[0].ID() != "file_rule" {
		t.Fatalf("got %+v", rules)
	}
}

func (s *stubObjectStorage) Shutdown(context.Context) error { return nil }
