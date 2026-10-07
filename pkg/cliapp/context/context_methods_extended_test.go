package context

import (
	"testing"
)

func TestContext_DerivationsAndLayers(t *testing.T) {
	ctx := &Context{
		Format:      "table",
		Verbose:     false,
		Quiet:       false,
		Profile:     "human",
		ProjectRoot: "/my/proj",
		layers:      make(map[ContextLayer]map[string]any),
	}

	layers := ctx.GetLayers()
	if layers == nil {
		t.Errorf("expected non-nil layers map")
	}

	// Derive variations
	dFmt := ctx.WithFormat("json")
	if dFmt.Format != "json" {
		t.Errorf("expected format json, got %s", dFmt.Format)
	}

	dVerb := ctx.WithVerbose(true)
	if !dVerb.Verbose {
		t.Errorf("expected verbose true")
	}

	dQuiet := ctx.WithQuiet(true)
	if !dQuiet.Quiet {
		t.Errorf("expected quiet true")
	}

	dProf := ctx.WithProfile("ai-agent")
	if dProf.Profile != "ai-agent" {
		t.Errorf("expected profile ai-agent, got %s", dProf.Profile)
	}

	dStorage := ctx.WithStorageContext(200, 100, true, 50)
	if dStorage.StorageMaxPageSize != 200 || dStorage.StorageDefaultPageSize != 100 || !dStorage.StorageEnableGrouping || dStorage.StorageMaxGroupSize != 50 {
		t.Errorf("unexpected storage context: %+v", dStorage)
	}

	// Test Derive with map
	dMap := ctx.Derive(map[string]any{
		"format":  "yaml",
		"verbose": true,
	})
	if dMap.Format != "yaml" || !dMap.Verbose {
		t.Errorf("unexpected derived map values: %+v", dMap)
	}
}

func TestContextBuilder_And_NodeAdapter(t *testing.T) {
	bldr := NewContextBuilder(ModeHierarchical)

	rootCtx := &Context{Format: "table"}
	bldr.SetRoot(rootCtx, "root")

	childCtx := &Context{Format: "json"}
	bldr.AddChild(childCtx, "child")

	nestedCtx := &Context{Format: "yaml"}
	_, err := bldr.AddNestedChild("child", nestedCtx, "nested")
	if err != nil {
		t.Fatalf("AddNestedChild failed: %v", err)
	}

	node := bldr.GetNode("child")
	if node == nil {
		t.Errorf("expected to find node 'child'")
	}

	bldr.SetMode(ModeSequential)

	// NodeAdapter
	nodeObj := &ContextNode{Context: rootCtx, Name: "rootNode"}
	adapter := NewContextNodeAdapter(nodeObj, 5, 1)
	if adapter.GetNode() != nodeObj {
		t.Errorf("expected adapter to return nodeObj")
	}
	if adapter.GetPrecedence() != 5 {
		t.Errorf("expected precedence 5, got %d", adapter.GetPrecedence())
	}
	adapter.SetPrecedence(10)
	if adapter.GetPrecedence() != 10 {
		t.Errorf("expected precedence 10 after set")
	}
}

func TestChainAdapter(t *testing.T) {
	ctx := &Context{Format: "table"}
	chain := NewContextChainAdapter(ctx, 1, 2)
	if chain.GetContext() != ctx {
		t.Errorf("expected context returned")
	}
	if chain.GetPrecedence() != 1 {
		t.Errorf("expected precedence 1")
	}
	if chain.GetDepth() != 2 {
		t.Errorf("expected depth 2")
	}
	chain.SetPrecedence(3)
	chain.SetDepth(4)
	if chain.GetPrecedence() != 3 || chain.GetDepth() != 4 {
		t.Errorf("unexpected precedence or depth after set")
	}
}
