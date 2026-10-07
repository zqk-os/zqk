package cli

import (
	"testing"

	"github.com/spf13/cobra"
	clictx "github.com/zqk-os/zqk/pkg/cliapp/context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestProcessor_LifecycleAndContextMethods(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cliapp.processor.lifecycle",
	})
	tempRoot := proj.Root
	cmd := &cobra.Command{Use: "test-proc"}
	cmd.SetContext(pkgctx.NewSystemContext())
	cliCtx := ContextForProjectRoot(tempRoot)
	cliCtx.Verbose = true
	cliCtx.Quiet = false
	SetContext(cmd, cliCtx)

	proc, err := NewProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	if proc.Context() != cliCtx {
		t.Errorf("unexpected context returned")
	}
	if proc.Storage() == nil {
		t.Errorf("expected non-nil storage")
	}
	if proc.SecurityContext() == nil {
		t.Errorf("expected non-nil security context")
	}
	if proc.ResolvedSecurityContext() == nil {
		t.Errorf("expected non-nil resolved security context")
	}
	if proc.ProjectRoot() != tempRoot {
		t.Errorf("expected %s, got %s", tempRoot, proc.ProjectRoot())
	}
	if proc.ResolvedProjectRoot() != tempRoot {
		t.Errorf("expected resolved root %s, got %s", tempRoot, proc.ResolvedProjectRoot())
	}
	if proc.StorageContext() == nil {
		t.Errorf("expected non-nil storage context")
	}
	if proc.ResolvedStorageContext() == nil {
		t.Errorf("expected non-nil resolved storage context")
	}
	if proc.Format() != FormatTable {
		t.Errorf("expected FormatTable, got %s", proc.Format())
	}
	if !proc.IsVerbose() {
		t.Errorf("expected IsVerbose=true")
	}
	if proc.IsQuiet() {
		t.Errorf("expected IsQuiet=false")
	}
	if proc.GetStorageProvider() == nil {
		t.Errorf("expected non-nil storage provider from deprecated getter")
	}

	// WithCLIOperation
	cliOpCtx := proc.WithCLIOperation()
	if cliOpCtx == nil {
		t.Errorf("expected non-nil CLI operation context")
	}

	// ValidateObject
	if err := proc.ValidateObject(map[string]any{"id": "GOAL-1"}, "goal", "planned"); err != nil {
		t.Errorf("expected nil error on valid object: %v", err)
	}
	if err := proc.ValidateObject(nil, "goal", "planned"); err == nil {
		t.Errorf("expected error when validating nil object")
	}
	if err := proc.ValidateObject(map[string]any{}, "", "planned"); err == nil {
		t.Errorf("expected error when kind is empty")
	}

	// CheckPermission
	if err := proc.CheckPermission("read", "goal"); err != nil {
		t.Errorf("expected nil error for CheckPermission")
	}

	// System objects
	builtInObj := map[string]any{
		objects.FieldKeyKind: "goal",
		objects.FieldKeyMetadata: map[string]any{
			"built_in": true,
		},
	}
	normalObj := map[string]any{
		objects.FieldKeyKind: "goal",
	}
	if !proc.IsSystemObject(builtInObj) {
		t.Errorf("expected built-in object to report IsSystemObject=true")
	}
	if proc.IsSystemObject(normalObj) {
		t.Errorf("expected normal object to report IsSystemObject=false")
	}
	if !proc.CanModifySystemObject(normalObj) {
		t.Errorf("expected normal object to be modifiable")
	}

	// Effective value
	v, ok := proc.GetEffectiveValue(processorContextKeyFormat)
	if !ok || v != "table" {
		t.Errorf("expected effective format 'table', got %v (ok=%v)", v, ok)
	}
	v, ok = proc.GetEffectiveValue(processorContextKeyVerbose)
	if !ok || v != true {
		t.Errorf("expected effective verbose true, got %v", v)
	}
	_, ok = proc.GetEffectiveValue("non_existent_key")
	if ok {
		t.Errorf("expected false for unknown key")
	}

	// ContextPrecedenceInfo
	info := proc.GetContextPrecedenceInfo()
	if info[processorPrecedenceOrderKey] == "" {
		t.Errorf("expected precedence order info")
	}

	// DeriveContext & WithFormat & WithStorageSettings
	derivedProc := proc.WithFormat("json")
	if derivedProc.Format() != FormatJSON {
		t.Errorf("expected derived format JSON, got %s", derivedProc.Format())
	}

	storageProc := proc.WithStorageSettings(100, 50, true, 20)
	sc := storageProc.StorageContext()
	if sc.MaxPageSize != 100 || sc.DefaultPageSize != 50 || !sc.EnableGrouping || sc.MaxGroupSize != 20 {
		t.Errorf("unexpected storage settings: %+v", sc)
	}
}

func TestProcessor_BuildContextSequentialAndTree(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cliapp.processor.builder",
	})
	tempRoot := proj.Root
	cmd := &cobra.Command{Use: "test-builder"}
	cmd.SetContext(pkgctx.NewSystemContext())
	cliCtx := ContextForProjectRoot(tempRoot)
	SetContext(cmd, cliCtx)

	proc, err := NewProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	// BuildContextSequential
	ctx1 := ContextForProjectRoot(tempRoot).WithFormat("table")
	ctx2 := ContextForProjectRoot(tempRoot).WithFormat("json")
	merged, err := proc.BuildContextSequential([]*Context{ctx1, ctx2})
	if err != nil {
		t.Fatalf("BuildContextSequential failed: %v", err)
	}
	if merged == nil || merged.Format != FormatJSON {
		t.Errorf("expected merged format JSON")
	}

	// BuildContextHierarchical
	node := &clictx.ContextNode{
		Context: ctx1.Context,
		Children: []*clictx.ContextNode{
			{Context: ctx2.Context},
		},
	}
	hierarchical, err := proc.BuildContextHierarchical(node)
	if err != nil {
		t.Fatalf("BuildContextHierarchical failed: %v", err)
	}
	if hierarchical == nil {
		t.Errorf("expected non-nil hierarchical context")
	}

	// BuildContextHybrid
	hybrid, err := proc.BuildContextHybrid(node)
	if err != nil {
		t.Fatalf("BuildContextHybrid failed: %v", err)
	}
	if hybrid == nil {
		t.Errorf("expected non-nil hybrid context")
	}

	// NewContextBuilder
	bldr := NewContextBuilder(clictx.ModeSequential)
	if bldr == nil {
		t.Errorf("expected non-nil ContextBuilder")
	}
}

func TestProcessor_CacheInvalidation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cliapp.processor.cache",
	})
	tempRoot := proj.Root
	cmd := &cobra.Command{Use: "test-cache"}
	cmd.SetContext(pkgctx.NewSystemContext())
	cliCtx := ContextForProjectRoot(tempRoot)
	SetContext(cmd, cliCtx)

	proc, err := NewProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	// InvalidateCache with nil context
	_, err = proc.InvalidateCache(nil)
	if err == nil {
		t.Errorf("expected error for nil cache invalidation context")
	}

	// InvalidateCache with empty IDs
	cacheCtx := pkgctx.NewCacheInvalidationContext([]string{}, tempRoot, "test-reason")
	count, err := proc.InvalidateCache(cacheCtx)
	if err != nil || count != 0 {
		t.Errorf("expected 0, nil for empty IDs")
	}

	// Handlers
	RegisterCacheInvalidationHandler(func(ids []string, projectRoot string) int {
		return len(ids)
	})
	RegisterCacheFreshnessHandler(func(projectRoot, reason, triggerOperation string, affectedKinds []string) int {
		return 1
	})
}
