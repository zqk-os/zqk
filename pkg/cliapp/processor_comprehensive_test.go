package cli

import (
	"strings"
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
	cliCtx.CacheFreshnessEnabled = true
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

	// InvalidateCacheAsync with nil
	resCh := proc.InvalidateCacheAsync(nil)
	res := <-resCh
	if res.State != pkgctx.StateFailed {
		t.Errorf("expected StateFailed for nil InvalidateCacheAsync")
	}

	// InvalidateCache with IDs
	cacheCtxValid := pkgctx.NewCacheInvalidationContext([]string{"GOAL-1", "GOAL-2"}, tempRoot, "test-update")
	cnt, err := proc.InvalidateCache(cacheCtxValid)
	if err != nil {
		t.Errorf("unexpected error on InvalidateCache: %v", err)
	}
	_ = cnt

	// CheckCacheFreshness
	if _, err := proc.CheckCacheFreshness(nil); err == nil {
		t.Errorf("expected error for nil cache freshness context")
	}
	freshCtx := pkgctx.NewCacheFreshnessContext(tempRoot, "test-freshness", "test-op")
	_, err = proc.CheckCacheFreshness(freshCtx)
	if err != nil {
		t.Errorf("CheckCacheFreshness unexpected error: %v", err)
	}

	// CheckCacheFreshnessAsync
	asyncCh := proc.CheckCacheFreshnessAsync(freshCtx)
	if asyncCh == nil {
		t.Errorf("expected non-nil channel from CheckCacheFreshnessAsync")
	}
	_ = <-asyncCh

	// TriggerCacheFreshnessCheck with configured triggers
	proc.cliCtx.CacheFreshnessTriggers = []string{"create", "update"}
	proc.TriggerCacheFreshnessCheck("delete", []string{"goal"}) // not in trigger list
	proc.TriggerCacheFreshnessCheck("create", []string{"goal"}) // in trigger list
	proc.cliCtx.CacheFreshnessTriggers = []string{"*"}
	proc.TriggerCacheFreshnessCheck("custom-op", []string{"goal"}) // wildcard match

	// Disabled cache freshness
	proc.cliCtx.CacheFreshnessEnabled = false
	c, _ := proc.CheckCacheFreshness(freshCtx)
	if c != 0 {
		t.Errorf("expected 0 from disabled CheckCacheFreshness")
	}
	disabledCh := proc.CheckCacheFreshnessAsync(freshCtx)
	_ = <-disabledCh
	proc.TriggerCacheFreshnessCheck("any", nil)
}

func TestProcessor_GovernorAndContextAccessors(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cliapp.processor.accessors",
	})
	tempRoot := proj.Root
	cmd := &cobra.Command{Use: "status"}
	cmd.SetContext(pkgctx.NewSystemContext())
	cliCtx := ContextForProjectRoot(tempRoot)
	cliCtx.Profile = "human"
	cliCtx.PriorityPlan = "PRI-1"
	cliCtx.Workstream = "WS-1"
	SetContext(cmd, cliCtx)

	proc, err := NewProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}
	defer proc.Close()

	// CheckGovernorApproval on normal command -> nil
	if err := proc.CheckGovernorApproval(cmd); err != nil {
		t.Errorf("expected nil for CheckGovernorApproval on non-governor command: %v", err)
	}

	// Logger, StorageFactory, OperationContext
	if proc.Logger() == nil {
		t.Errorf("expected non-nil Logger")
	}
	if proc.StorageFactory() == nil {
		t.Errorf("expected non-nil StorageFactory")
	}
	if proc.OperationContext() == nil {
		t.Errorf("expected non-nil OperationContext")
	}

	// GetContextLayers and GetEffectiveValue
	layers := proc.GetContextLayers()
	if layers == nil {
		t.Errorf("expected non-nil ContextLayers")
	}

	for _, key := range []string{
		"format", "verbose", "quiet", "profile", "project_root", "priority_plan", "workstream",
		"milestone", "storage.max_page_size", "storage.default_page_size", "storage.enable_grouping", "storage.max_group_size",
	} {
		val, ok := proc.GetEffectiveValue(key)
		if !ok || val == nil {
			t.Errorf("expected ok=true for key %s", key)
		}
	}
	if _, ok := proc.GetEffectiveValue("non_existent_key"); ok {
		t.Errorf("expected ok=false for non_existent_key")
	}

	// System object check and modification
	nonSys := map[string]any{"id": "GOAL-1", "kind": "goal"}
	if proc.IsSystemObject(nonSys) {
		t.Errorf("expected false for IsSystemObject on nonSys")
	}
	if !proc.CanModifySystemObject(nonSys) {
		t.Errorf("expected true for CanModifySystemObject on nonSys")
	}

	// ValidateObject
	if err := proc.ValidateObject(nil, "goal", ""); err == nil {
		t.Errorf("expected error for nil object")
	}
	if err := proc.ValidateObject(nonSys, "", ""); err == nil {
		t.Errorf("expected error for empty kind")
	}
	if err := proc.ValidateObject(nonSys, "goal", ""); err != nil {
		t.Errorf("unexpected error for valid object: %v", err)
	}

	// CheckPermission
	if err := proc.CheckPermission("read", "goal"); err != nil {
		t.Errorf("expected nil from CheckPermission: %v", err)
	}

	// StorageTuple and WithProcessor
	opCtx, sCtx, sp := proc.StorageTuple()
	if opCtx == nil || sCtx == nil || sp == nil {
		t.Errorf("expected non-nil StorageTuple elements")
	}
	wrappedHandler := WithProcessor(func(c *cobra.Command, a []string, p *Processor) error {
		return nil
	})
	if wrappedHandler == nil {
		t.Errorf("expected non-nil wrapped handler")
	}
}

func TestProcessor_CheckGovernorApproval_WithRequirements(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cliapp.processor.governor",
	})
	cmd := &cobra.Command{Use: "test-governor"}
	cmd.SetContext(pkgctx.NewSystemContext())
	RequireGovernorApproval(cmd, true)
	cmd.Flags().String("event-id", "", "")

	cliCtx := ContextForProjectRoot(proj.Root)
	SetContext(cmd, cliCtx)

	proc, err := NewProcessor(cmd)
	if err != nil {
		t.Fatalf("NewProcessor failed: %v", err)
	}

	// 1. Without event-id -> creates proposed event (or fails validation) and returns error
	err = proc.CheckGovernorApproval(cmd)
	if err == nil {
		t.Fatalf("expected error from CheckGovernorApproval without event-id")
	}

	// 2. With event-id on file storage (non-graph) -> returns graph backend required error
	_ = cmd.Flags().Set("event-id", "EVT-123")
	err = proc.CheckGovernorApproval(cmd)
	if err == nil || !strings.Contains(err.Error(), "graph backend required") {
		t.Fatalf("expected graph backend required error, got: %v", err)
	}
}

func TestWithProcessor_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cliapp.processor.withproc",
	})
	cmd := &cobra.Command{Use: "run-with-proc"}
	cmd.SetContext(pkgctx.NewSystemContext())
	cliCtx := ContextForProjectRoot(proj.Root)
	SetContext(cmd, cliCtx)

	invoked := false
	wrapped := WithProcessor(func(c *cobra.Command, args []string, p *Processor) error {
		invoked = true
		if p == nil {
			t.Errorf("expected non-nil Processor")
		}
		return nil
	})

	if err := wrapped(cmd, nil); err != nil {
		t.Fatalf("wrapped run failed: %v", err)
	}
	if !invoked {
		t.Errorf("expected wrapped function to be invoked")
	}
}
