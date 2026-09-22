// BLI-STARTER-COMMUNITY-037 / PRI-STARTER-COMMUNITY-037 coverage elevation
package observer

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type batchWriter struct {
	mockGraphConn
	batchErr error
	batchRes *provider.BatchResult
}

func (b *batchWriter) ExecuteBatch(_ context.Context, operations []provider.Operation) (*provider.BatchResult, error) {
	if b.batchErr != nil {
		return nil, b.batchErr
	}
	if b.batchRes != nil {
		return b.batchRes, nil
	}
	return &provider.BatchResult{SuccessCount: len(operations)}, nil
}

type errLLM struct {
	mockLLMClient
	intentErr error
	embedErr  error
}

func (e *errLLM) GenerateIntent(context.Context, string) (string, error) {
	if e.intentErr != nil {
		return "", e.intentErr
	}
	return e.intentReturn, nil
}

func (e *errLLM) GenerateEmbedding(context.Context, string) ([]float32, error) {
	if e.embedErr != nil {
		return nil, e.embedErr
	}
	return e.embeddingReturn, nil
}

func TestExtraEntityToNodeAndIDs(t *testing.T) {
	e := &Entity{
		Kind: "method", Name: "Do", File: "a.go", Line: 9, Receiver: "Bar",
		Signature: "func (Bar) Do()", Language: "go", Intent: "work",
		Embedding: []float32{0.1}, Metadata: map[string]string{"doc": "docs"},
	}
	n := EntityToNode(e)
	if n.Properties["receiver"] != "Bar" || n.Properties[objects.FieldKeyIntent] != "work" {
		t.Fatalf("props = %#v", n.Properties)
	}
	if _, ok := n.Properties["embedding"]; !ok {
		t.Fatal("missing embedding")
	}
	pkg := PackageNode("github.com/zqk-os/zqk/pkg/observer")
	if pkg.ID != PackageID("github.com/zqk-os/zqk/pkg/observer") {
		t.Fatalf("package id = %s", pkg.ID)
	}
	if SourceFileNode("a.go").ID != FileID("a.go") {
		t.Fatal("file id mismatch")
	}
}

func TestExtraPopulateEdgesAndErrors(t *testing.T) {
	ctx := context.Background()
	res, err := Populate(ctx, &mockGraphConn{}, nil, "run")
	if err != nil || res.NodesCreated != 0 {
		t.Fatalf("nil result: %+v %v", res, err)
	}

	result := &ExtractResult{Entities: []Entity{
		{Kind: "type", Name: "Bar", File: "a.go", Line: 1, Language: "go", Imports: []string{"fmt"}},
		{Kind: "function", Name: "Foo", File: "a.go", Line: 5, Language: "go", Calls: []string{"Bar"}, DependsOn: []string{"Bar"}},
		{Kind: "method", Name: "Do", File: "a.go", Line: 7, Receiver: "Bar", Language: "go", Imports: []string{"fmt"}},
	}}
	mock := &mockGraphConn{}
	got, err := Populate(ctx, mock, result, "extract-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.NodesCreated < 4 || got.EdgesCreated < 4 {
		t.Fatalf("created nodes=%d edges=%d", got.NodesCreated, got.EdgesCreated)
	}
	var imports, calls, depends int
	for _, e := range mock.Edges {
		switch e.Type {
		case EdgeImports:
			imports++
		case EdgeCalls:
			calls++
		case EdgeDependsOn:
			depends++
		}
	}
	if imports == 0 || calls == 0 || depends == 0 {
		t.Fatalf("edge types imports=%d calls=%d depends=%d", imports, calls, depends)
	}

	failNode := &mockGraphConn{FailOp: "create_node"}
	failRes, err := Populate(ctx, failNode, result, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(failRes.Errors) == 0 {
		t.Fatal("expected create_node error recorded")
	}
	failEdge := &mockGraphConn{FailOp: "create_edge"}
	failEdgeRes, err := Populate(ctx, failEdge, result, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(failEdgeRes.Errors) == 0 {
		t.Fatal("expected create_edge error recorded")
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Populate(canceled, &mockGraphConn{}, result, "x"); err == nil {
		t.Fatal("expected canceled populate error")
	}
}

func TestExtraPopulateBatch(t *testing.T) {
	ctx := context.Background()
	if got, err := PopulateBatch(ctx, nil, nil, ""); err != nil || got.NodesCreated != 0 {
		t.Fatalf("nil conn/result: %+v %v", got, err)
	}

	result := &ExtractResult{Entities: []Entity{
		{Kind: "struct", Name: "Bar", File: "a.go", Line: 1, Language: "go", Imports: []string{"fmt"}},
		{Kind: "method", Name: "Do", File: "a.go", Line: 2, Receiver: "Bar", Language: "go", Calls: []string{"Do"}, DependsOn: []string{"Bar"}},
	}}
	ok := &batchWriter{}
	got, err := PopulateBatch(ctx, ok, result, "batch-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.NodesCreated < 3 || got.EdgesCreated < 3 {
		t.Fatalf("batch created nodes=%d edges=%d", got.NodesCreated, got.EdgesCreated)
	}

	withErrs := &batchWriter{batchRes: &provider.BatchResult{Errors: []error{errors.New("edge fail")}}}
	got, err = PopulateBatch(ctx, withErrs, result, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Errors) == 0 {
		t.Fatal("expected batch errors collected")
	}

	fallback := &batchWriter{batchErr: errors.New("batch down")}
	got, err = PopulateBatch(ctx, fallback, result, "fb")
	if err != nil {
		t.Fatal(err)
	}
	if got.NodesCreated == 0 {
		t.Fatal("expected fallback Populate to create nodes")
	}
}

func TestExtraSearchAndTokens(t *testing.T) {
	t.Parallel()
	if toks := SearchTokens(""); toks != nil {
		t.Fatalf("empty tokens = %v", toks)
	}
	if toks := SearchTokens("the and for add"); len(toks) != 0 {
		t.Fatalf("stopwords = %v", toks)
	}
	toks := SearchTokens("alpha bravo charlie delta echo foxtrot golf")
	if len(toks) != 5 {
		t.Fatalf("cap tokens = %v", toks)
	}
	if InferSourcePath("nothing here") != "" {
		t.Fatal("expected empty infer")
	}
	if got := InferSourcePath("see cmd/zqk/main.go,"); got != "cmd/zqk/main.go" {
		t.Fatalf("infer = %q", got)
	}
	line := FormatHit(Entity{Name: "Foo", Kind: "function", File: "a.go", Line: 1})
	if !strings.Contains(line, "Foo") {
		t.Fatal(line)
	}

	root := t.TempDir()
	writeGo(t, filepath.Join(root, "pkg", "foo", "a.go"), "package foo\n\nfunc Alpha() {}\ntype Beta struct{}\n")
	writeGo(t, filepath.Join(root, "pkg", "foo", "vendor", "skip.go"), "package skip\n\nfunc Hidden() {}\n")
	writeGo(t, filepath.Join(root, "cmd", "zqk", "main.go"), "package main\n\nfunc MainFn() {}\n")

	hits, err := Search(context.Background(), Query{Root: root, Path: "pkg/foo", Kind: "function", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Name != "Alpha" {
		t.Fatalf("dir+kind+limit = %#v", hits)
	}
	hits, err = Search(context.Background(), Query{Root: root, Name: "MainFn", Limit: 99})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Name != "MainFn" {
		t.Fatalf("name search = %#v", hits)
	}
	if _, err := Search(context.Background(), Query{Root: root, Path: "pkg/foo/a.go.txt"}); err == nil {
		t.Fatal("expected non-go path error")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Search(canceled, Query{Root: root, Name: "Alpha"}); err == nil {
		t.Fatal("expected canceled search")
	}

	if (ExtractError{Msg: "boom"}).Error() != "boom" {
		t.Fatal("extract error without file")
	}
	if got := (ExtractError{File: "a.go", Msg: "boom"}).Error(); got != "a.go: boom" {
		t.Fatalf("extract error = %s", got)
	}
}

func TestExtraSemanticEnhance(t *testing.T) {
	se := NewSemanticEnhancer(context.Background(), &mockLLMClient{intentReturn: "i", embeddingReturn: []float32{1}}, nil)
	if err := se.Enhance(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := se.Enhance(context.Background(), &ExtractResult{}); err != nil {
		t.Fatal(err)
	}
	if err := se.Enhance(context.Background(), &ExtractResult{Entities: []Entity{{Kind: "file", File: "a.go"}}}); err != nil {
		t.Fatal(err)
	}

	t.Setenv(zqkenv.ObserverConcurrency().Name(), "0")
	t.Setenv(zqkenv.ObserverSkipIntent().Name(), "true")
	result := &ExtractResult{Entities: []Entity{{
		Kind: "function", Name: "Real", File: "real.go", Signature: "func Real()",
		Metadata: map[string]string{"doc": "docs"},
	}}}
	if err := se.Enhance(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entities[0].Embedding) == 0 {
		t.Fatal("expected embedding with skip-intent")
	}

	t.Setenv(zqkenv.ObserverIncludeTests().Name(), "true")
	t.Setenv(zqkenv.ObserverSkipIntent().Name(), "false")
	withTests := &ExtractResult{Entities: []Entity{
		{Kind: "function", Name: "T", File: "foo_test.go", Signature: "func T()"},
		{Kind: "function", Name: "M", File: "mock_foo.go", Signature: "func M()"},
	}}
	if err := se.Enhance(context.Background(), withTests); err != nil {
		t.Fatal(err)
	}

	failing := NewSemanticEnhancer(context.Background(), &errLLM{
		mockLLMClient: mockLLMClient{embeddingReturn: []float32{1}},
		intentErr:     errors.New("intent boom"),
		embedErr:      errors.New("embed boom"),
	}, nil)
	failResult := &ExtractResult{Entities: []Entity{
		{Kind: "type", Name: "A", File: "a.go", Signature: "type A struct{}"},
		{Kind: "interface", Name: "B", File: "b.go", Signature: "type B interface{}"},
	}}
	if err := failing.Enhance(context.Background(), failResult); err == nil {
		t.Fatal("expected enhancement failure when majority embed fails")
	}

	em := NewObserverEmitter()
	got := make(chan ObserverEvent, 1)
	em.Subscribe(func(_ context.Context, ev ObserverEvent) { got <- ev })
	em.Emit(context.Background(), ObserverEvent{Type: EventExtractStarted})
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("emitter timeout")
	}
	NotifyAgentConnection(context.Background(), AgentConnectionInfo{ClientID: "c1", ClientName: "n", Version: "1"})
}

// keep llm.Client compile-checked for the extra mock
var _ llm.Client = (*errLLM)(nil)
