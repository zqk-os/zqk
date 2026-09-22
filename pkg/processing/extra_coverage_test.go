// BLI-STARTER-COMMUNITY-034 / PRI-STARTER-COMMUNITY-034 coverage elevation
package processing

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLoadReferenceFile_InferAndErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing := filepath.Join(dir, "no.yaml")
	if _, err := LoadReferenceFile(missing); err == nil {
		t.Fatal("missing")
	}
	bad := filepath.Join(dir, "bad.yaml")
	if err := fileutil.WriteFile(bad, []byte(":\n["), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReferenceFile(bad); err == nil {
		t.Fatal("bad yaml")
	}
	empty := filepath.Join(dir, "empty.yaml")
	if err := fileutil.WriteFile(empty, []byte("metadata: {}\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReferenceFile(empty); err == nil {
		t.Fatal("unknown format")
	}
	ops := filepath.Join(dir, "ops.yaml")
	if err := fileutil.WriteFile(ops, []byte("operations:\n  - type: create\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadReferenceFile(ops)
	if err != nil || got.Format != formatOperations {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestExpandTemplate_Variables(t *testing.T) {
	t.Parallel()
	tpl := &Template{
		Variables: []map[string]any{{"name": "n1"}},
		Template: map[string]any{
			"title":  "${name}",
			"keep":   "${missing}",
			"plain":  "x",
			"nested": map[string]any{"k": "${name}"},
			"list":   []any{"${name}", "z", 1},
			"n":      3,
		},
	}
	out, err := tpl.ExpandTemplate()
	if err != nil || len(out) != 1 || out[0]["title"] != "n1" {
		t.Fatalf("%v %v", out, err)
	}
}

func TestProcessReferenceFile_DryRunAndOps(t *testing.T) {
	t.Parallel()
	p := NewProcessor(storage.NewNoopObjectStorage(), "test")
	ctx := context.Background()
	if _, err := p.ProcessReferenceFile(ctx, &ReferenceFile{Format: "nope"}, false); err == nil {
		t.Fatal("unsupported")
	}
	dry, err := p.ProcessReferenceFile(ctx, &ReferenceFile{
		Format: formatOperations,
		Operations: []Operation{
			{Type: opTypeCreate, Description: "c"},
			{Type: opTypeList, Kind: "backlog_item"},
		},
	}, true)
	if err != nil || dry.Operations[0].Status != statusWouldExecute {
		t.Fatalf("%+v %v", dry, err)
	}
	dataDry, err := p.ProcessReferenceFile(ctx, &ReferenceFile{
		Format: formatData,
		Data:   []map[string]any{{"id": "O-1"}},
	}, true)
	if err != nil || dataDry.SuccessCount != 1 {
		t.Fatalf("%+v %v", dataDry, err)
	}
	created, err := p.ProcessReferenceFile(ctx, &ReferenceFile{
		Format: formatOperations,
		Operations: []Operation{
			{Type: opTypeCreate, Object: map[string]any{"id": "O-1"}},
			{Type: opTypeUpdate, ContinueOnError: true},
			{Type: opTypeDelete, ContinueOnError: true},
			{Type: opTypeGet, ContinueOnError: true},
			{Type: "unknown", ContinueOnError: true},
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if created.FailureCount == 0 {
		t.Fatalf("want failures %+v", created)
	}
	_, err = p.ProcessReferenceFile(ctx, &ReferenceFile{
		Format: formatTemplate,
		Template: &Template{
			Variables: []map[string]any{{"t": "x"}},
			Template:  map[string]any{"title": "${t}"},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
}

type listOK struct{ storage.NoopObjectStorage }

func (listOK) List(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, storage.ListFilter) (*storage.QueryResult, error) {
	return &storage.QueryResult{}, nil
}

func TestProcess_ListSuccess(t *testing.T) {
	t.Parallel()
	p := NewProcessor(listOK{}, "test")
	res, err := p.ProcessReferenceFile(context.Background(), &ReferenceFile{
		Format:     formatOperations,
		Operations: []Operation{{Type: opTypeList, Kind: "goal"}},
	}, false)
	if err != nil || res.SuccessCount != 1 {
		t.Fatalf("%+v %v", res, err)
	}
}

type crudStub struct {
	storage.NoopObjectStorage
	objs map[string]map[string]any
}

func (c *crudStub) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if o, ok := c.objs[id]; ok {
		return o, nil
	}
	return nil, storage.ErrNoopObjectStorage
}

func (c *crudStub) Update(_ context.Context, _ *pkgctx.SecurityContext, id string, updates map[string]any) error {
	c.objs[id] = updates
	return nil
}

func (c *crudStub) Delete(context.Context, *pkgctx.SecurityContext, string, bool) error { return nil }

func (c *crudStub) BulkCreate(_ context.Context, _ *pkgctx.SecurityContext, objects []map[string]any) (*storage.BulkResult, error) {
	res := &storage.BulkResult{SuccessCount: len(objects), TotalCount: len(objects), Results: objects}
	return res, nil
}

func TestProcess_CRUDAndBulkAndHardFail(t *testing.T) {
	t.Parallel()
	stub := &crudStub{objs: map[string]map[string]any{"O-2": {"id": "O-2", "a": 1}}}
	p := NewProcessor(stub, "test")
	ctx := context.Background()
	res, err := p.ProcessReferenceFile(ctx, &ReferenceFile{
		Format: formatOperations,
		Operations: []Operation{
			{Type: opTypeUpdate, ID: "O-2", Updates: map[string]any{"a": 2}},
			{Type: opTypeDelete, ID: "O-2"},
			{Type: opTypeGet, ID: "O-2"},
			{Type: opTypeList, ContinueOnError: true},
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.SuccessCount < 3 {
		t.Fatalf("%+v", res)
	}
	bulk, err := p.ProcessReferenceFile(ctx, &ReferenceFile{
		Format: formatData,
		Data:   []map[string]any{{"id": "O-9"}},
	}, false)
	if err != nil || bulk.SuccessCount != 1 {
		t.Fatalf("%+v %v", bulk, err)
	}
	_, err = p.ProcessReferenceFile(ctx, &ReferenceFile{
		Format:     formatOperations,
		Operations: []Operation{{Type: "unknown"}},
	}, false)
	if err == nil {
		t.Fatal("hard fail")
	}
	_, err = NewProcessor(storage.NewNoopObjectStorage(), "test").ProcessReferenceFile(ctx, &ReferenceFile{
		Format: formatData,
		Data:   []map[string]any{{"id": "x"}},
	}, false)
	if err == nil {
		t.Fatal("bulk noop")
	}
}
