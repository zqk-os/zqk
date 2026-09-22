// BLI-STARTER-COMMUNITY-056 / PRI-STARTER-COMMUNITY-056 coverage elevation
package context

import (
	"bytes"
	stdcontext "context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestExtraValidationQueryPipeline(t *testing.T) {
	vc := NewValidationContext(t.TempDir()).
		WithWorkers(2).
		WithMaxCacheAge(time.Second).
		WithTimeout(time.Second).
		WithEnabledTiers([]int{1}).
		WithEnabledKinds([]string{"backlog_item"}).
		WithCacheOnly(true).
		WithRefreshCache(true).
		WithContext(NewSystemContext())
	vc.SkipKinds = []string{"audit_event"}
	vc.CustomPriority = map[string]int{"id-1": 9, "backlog_item": 4}
	if !vc.IsTierEnabled(1) || vc.IsTierEnabled(2) {
		t.Fatal("tiers")
	}
	emptyTiers := NewValidationContext("r")
	emptyTiers.EnabledTiers = nil
	if !emptyTiers.IsTierEnabled(9) {
		t.Fatal("all tiers")
	}
	if vc.IsKindEnabled("audit_event") || !vc.IsKindEnabled("backlog_item") || vc.IsKindEnabled("other") {
		t.Fatal("kinds")
	}
	allKinds := NewValidationContext("r")
	if !allKinds.IsKindEnabled("anything") {
		t.Fatal("all kinds")
	}
	if vc.GetPriority("id-1", "x") != 9 || vc.GetPriority("x", "backlog_item") != 4 || vc.GetPriority("x", "y") != 2 {
		t.Fatal("priority")
	}
	_ = vc.GetContext()
	vc.SetState(StateProcessing)
	if vc.GetState() != StateProcessing {
		t.Fatal("state")
	}
	vc.SetPrecedence(1)
	vc.SetDepth(1)
	_ = vc.GetPrecedence()
	_ = vc.GetDepth()
	if vc.Merge(nil) != vc {
		t.Fatal("merge nil")
	}
	_ = vc.Merge(NewValidationContext("other"))
	if errs := NewValidationContext("").WithWorkers(0).WithMaxCacheAge(-1).Validate(); len(errs) < 2 {
		t.Fatalf("validate: %#v", errs)
	}

	q := NewQueryContext(nil, 0, 0, "", "", true)
	_ = q.GetEffectiveLimit()
	q.Compute()
	q.Compute() // already computed
	_ = q.ShouldPaginate()
	_ = q.ShouldGroup()
	_ = q.ShouldSort()
	if q.GetSortOrder() != sortOrderASC {
		t.Fatal("asc")
	}
	grouped := NewQueryContext(NewGroupingStorageContext(10), 9999, 5, "kind", "id", false)
	if !grouped.ShouldPaginate() || !grouped.ShouldGroup() || grouped.GetSortOrder() != sortOrderDESC {
		t.Fatal("grouped query")
	}

	ClearAllListeners()
	RegisterListener(StatePending, func(stdcontext.Context, any) (any, error) { return "ok", nil }, ListenerConfig{Name: "sync", Priority: 1, Required: true})
	RegisterListener(StatePending, func(stdcontext.Context, any) (any, error) { return "async", nil }, ListenerConfig{Name: "async", Async: true, Timeout: time.Second})
	if len(GetListeners(StatePending)) < 2 {
		t.Fatal("listeners")
	}
	ch := ProcessContext(vc)
	res, err := WaitForResult(ch, 5*time.Second)
	if err != nil || res == nil {
		t.Fatalf("process: %v %v", res, err)
	}
	ClearListeners(StatePending)
	ClearAllListeners()

	timed := make(chan *ProcessingResult)
	if _, err := WaitForResult(timed, time.Millisecond); err == nil {
		t.Fatal("timeout")
	}

	called := false
	ctx := WithValidationProgress(NewSystemContext(), func(string, string) { called = true })
	if fn := GetValidationProgress(ctx); fn == nil {
		t.Fatal("progress")
	} else {
		fn("phase", "msg")
	}
	if !called {
		t.Fatal("progress fn")
	}
	if GetValidationProgress(NewSystemContext()) != nil {
		t.Fatal("no progress")
	}
}

func TestExtraContextFlagsAndCache(t *testing.T) {
	SetMCPServerServing(true)
	if !IsMCPServerServing() || !GetMCPServerContext().IsServing() {
		t.Fatal("mcp serving")
	}
	GetMCPServerContext().SetServing(false)
	SetMCPServerServing(false)

	_ = ActorIDForAttribution("")
	_ = ActorIDForAttribution("acc-1")
	_ = IsSystemAccount("")
	_ = IsSystemAccount("system")

	sec := NewSecurityContext("acc-1", []string{"admin"}, []string{PermissionDeleteAll})
	_ = sec.GetRoles()
	sec.SetPrecedence(3)
	sec.SetDepth(2)
	_ = sec.GetPrecedence()
	_ = sec.GetDepth()
	_ = sec.GetLLMAPIKey("openai")
	_ = sec.GetLLMBaseURL("openai")
	_ = sec.Validate()
	_ = sec.Merge(nil)
	_ = sec.Merge(NewSystemSecurityContext())
	ctx := WithSecurityContext(NewSystemContext(), sec)
	if GetSecurityContext(ctx) == nil {
		t.Fatal("sec ctx")
	}
	_ = NewTestSecurityContext()
	if MayHardDeleteCoreWithoutReason(nil) || !MayHardDeleteCoreWithoutReason(sec) {
		t.Fatal("hard delete")
	}
	if MaySweepObjectDraftPlane(nil) || !MaySweepObjectDraftPlane(sec) {
		t.Fatal("sweep")
	}
	user := NewSecurityContext("u", []string{"doer"}, nil)
	if MayHardDeleteCoreWithoutReason(user) || MaySweepObjectDraftPlane(user) {
		t.Fatal("user elevation")
	}

	st := NewStorageContext()
	st.SetPrecedence(1)
	st.SetDepth(1)
	_ = st.GetPrecedence()
	_ = st.GetDepth()
	_ = st.Validate()
	_ = st.Merge(nil)
	_ = st.Merge(NewPaginationStorageContext(10, 5))
	RegisterStorageContextProvider(func() *StorageContext { return NewStorageContext() })
	_ = GetStorageContext()

	cli := NewCliInitializationContext(func(string) string { return "/tmp/root" }, ".")
	_ = cli.GetProjectRoot()
	cli.SetPrecedence(1)
	cli.SetDepth(1)
	_ = cli.GetPrecedence()
	_ = cli.GetDepth()
	_ = cli.Validate()
	_ = cli.Merge(nil)
	_ = cli.Merge(NewCliInitializationContext(func(string) string { return "" }, ""))

	bg := stdcontext.Background()
	ev := TrustedLifecycleEvent{EventID: "e1", TargetID: "t1", FromState: "a", ToState: "b"}
	tctx := WithTrustedLifecycleEvent(bg, ev)
	if _, ok := GetTrustedLifecycleEvent(tctx); !ok {
		t.Fatal("trusted event")
	}
	if !IsTrustedLifecycleEventTarget(tctx, "t1") || IsTrustedLifecycleEventTarget(tctx, "nope") {
		t.Fatal("trusted target")
	}
	_, _ = GetTrustedLifecycleEvent(nil)
	_ = IsTrustedLifecycleEventTarget(nil, "t1")

	bg2 := WithLifecycleBreakGlass(bg, "because")
	if GetLifecycleBreakGlassReason(bg2) == "" || !IsLifecycleBreakGlass(bg2) {
		t.Fatal("break glass")
	}
	_ = GetLifecycleBreakGlassReason(bg)
	_ = IsLifecycleBreakGlass(bg)
	_ = WithLifecycleBreakGlass(bg, "")

	pctx := WithPromoteOnCreate(bg)
	if !GetPromoteOnCreate(pctx) || GetPromoteOnCreate(nil) {
		t.Fatal("promote")
	}
	dctx := WithAllowCoreObjectDelete(bg)
	if !GetAllowCoreObjectDelete(dctx) || GetAllowCoreObjectDelete(nil) {
		t.Fatal("core delete")
	}

	var buf bytes.Buffer
	wctx := WithCommandOutputWriter(bg, &buf)
	if w, ok := GetCommandOutputWriterFromContext(wctx); !ok || w == nil {
		t.Fatal("writer")
	}
	_, _ = GetCommandOutputWriterFromContext(nil)
	rctx := WithStdinReader(bg, strings.NewReader("in"))
	if r, ok := GetStdinReaderFromContext(rctx); !ok || r == nil {
		t.Fatal("stdin")
	}
	_, _ = GetStdinReaderFromContext(nil)

	lctx := WithLifecycleProjectRoot(bg, t.TempDir())
	if GetLifecycleProjectRoot(lctx) == "" {
		t.Fatal("lifecycle root")
	}
	_ = WithLifecycleProjectRoot(nil, "")
	_ = GetLifecycleProjectRoot(nil)
	if !HasListCountSlotHeld(WithListCountSlotHeld(bg)) || HasListCountSlotHeld(nil) {
		t.Fatal("slot")
	}
	_ = WithListCountSlotHeld(nil)
	if !GetBypassCache(WithBypassCache(bg)) || GetBypassCache(nil) {
		t.Fatal("bypass")
	}
	_ = WithBypassCache(nil)

	to, cancel := EnforceTimeout(bg, time.Second)
	cancel()
	_ = to
	dead, dcancel := stdcontext.WithTimeout(bg, time.Second)
	to2, cancel2 := EnforceTimeout(dead, time.Minute)
	cancel2()
	dcancel()
	_ = to2

	if GetCacheContext(bg) != nil {
		t.Fatal("no cache ctx")
	}
	_ = WithCacheOperation(bg, nil)
	cctx := WithCacheUpdate(bg, "id", "kind", "path")
	if GetCacheContext(cctx) == nil {
		t.Fatal("cache update")
	}
	_ = WithCacheInvalidate(bg, "id")
	_ = WithCacheIDChange(bg, "old", "new", "kind", "p")
	_ = WithCacheMode(nil, CacheModeDefault)
	mctx := WithCacheMode(bg, CacheModeTestSync)
	if GetCacheMode(mctx) != CacheModeTestSync {
		t.Fatal("cache mode")
	}
	_ = GetCacheMode(nil)

	inv := NewCacheInvalidationContext([]string{"a"}, t.TempDir(), "test").WithContext(NewSystemContext())
	inv.SetState(StateCompleted)
	_ = inv.GetState()
	_ = inv.GetContext()
	fresh := NewCacheFreshnessContext(t.TempDir(), "r", "create").WithAffectedKinds([]string{"k"}).WithContext(NewSystemContext())
	fresh.SetState(StateFailed)
	_ = fresh.GetState()
	_ = fresh.GetContext()

	blk := NewBlockingCheckContext(t.TempDir(), "create", "backlog_item", "id-1").WithBypass(true).WithContext(NewSystemContext())
	blk.SetState(StatePending)
	_ = blk.GetState()
	_ = blk.GetContext()

	_, errs := BuildContextChain()
	if len(errs) == 0 {
		t.Fatal("empty chain")
	}
	chain, _ := BuildContextChain(NewSystemLoggingContext(), NewHumanLoggingContext())
	if chain == nil {
		t.Fatal("chain")
	}
	_, _ = BuildNestedContextChain(nil)
	nested, _ := BuildNestedContextChain(NewSystemLoggingContext(), NewHumanLoggingContext())
	if nested != nil {
		_ = ValidateContextChain(nested)
		_ = nested.GetChainHead()
		_ = nested.GetChainTail()
	}
	_, _ = MergeContexts()
	_, _ = MergeContexts(NewSystemLoggingContext(), NewHumanLoggingContext())
	SetPrecedenceForContext(NewSystemLoggingContext(), 1)
	SetDepthForContext(NewSystemLoggingContext(), 1)
	_ = ValidateContextChain(nil)
	head := NewContextChain(NewSystemLoggingContext())
	head.Append(NewHumanLoggingContext())
	head.Prepend(NewLoggingContext(ProfileDebug))
	head.AddChild(NewSystemLoggingContext())
	_, _ = head.ProcessBreadthFirst()

	ldc := NewLoggingDecisionContext().
		WithLoggingContext(NewHumanLoggingContext()).
		WithSecurityContext(sec).
		WithComponent("storage").
		WithOperation("create").
		WithClientID("c1").
		WithTraceEnabled(true).
		WithVerbose(true).
		WithOperationContext(NewSystemContext())
	_ = ldc.ShouldLog(3, "storage", "create")
	_ = ldc.ShouldLog(0, "other", "x")
	_ = ldc.GetLogLevel()
	_ = ldc.GetDestinations(1, t.TempDir())
	ldc.SetPrecedence(1)
	ldc.SetDepth(1)
	_ = ldc.GetPrecedence()
	_ = ldc.GetDepth()
	_ = ldc.Validate()
	_ = ldc.Merge(nil)
	other := NewLoggingDecisionContext()
	other.SetPrecedence(50)
	_ = ldc.Merge(other)
	_ = ldc.Merge(NewSystemLoggingContext())
	wrapped := WithLoggingDecisionContext(bg, ldc)
	if GetLoggingDecisionContext(wrapped) == nil {
		t.Fatal("ldc wrap")
	}
	_ = WithLoggingDecisionContext(bg, nil)
	_ = GetLoggingDecisionContext(bg)
	sysLDC := NewLoggingDecisionContext()
	_ = sysLDC.GetDestinations(1, t.TempDir())
	_ = sysLDC.GetDestinations(0, "")
	human := NewLoggingDecisionContext().WithLoggingContext(NewHumanLoggingContext())
	_ = human.GetDestinations(1, t.TempDir())
	mcpLDC := NewLoggingDecisionContext().WithLoggingContext(NewLoggingContext(ProfileMCP))
	GetMCPServerContext().SetServing(true)
	_ = mcpLDC.GetDestinations(1, t.TempDir())
	GetMCPServerContext().SetServing(false)
	_ = NewLoggingContext(ProfileAIAgent)
	_ = NewLoggingContext(ProfileDebug)
	lc := NewSystemLoggingContext().WithSuppressDebugToStdout(true).WithLevel(1)
	_ = lc.GetWriter()
	_ = lc.ShouldSuppressDebugToStdout()
	lwrap := WithLoggingContext(bg, lc)
	if GetLoggingContext(lwrap) == nil {
		t.Fatal("logging wrap")
	}

	_ = io.Discard
}

func TestExtraPipelineListenersAndLLM(t *testing.T) {
	t.Cleanup(ClearAllListeners)
	ClearAllListeners()
	RegisterListener(StatePending, func(stdcontext.Context, any) (any, error) {
		return "sync-ok", nil
	}, ListenerConfig{Name: "sync-ok", Priority: 0})
	RegisterListener(StatePending, func(stdcontext.Context, any) (any, error) {
		return nil, errFmt("optional")
	}, ListenerConfig{Name: "optional-fail", Priority: 1})
	RegisterListener(StatePending, func(stdcontext.Context, any) (any, error) {
		return "async-ok", nil
	}, ListenerConfig{Name: "async-ok", Async: true, Timeout: time.Second})
	RegisterListener(StatePending, func(stdcontext.Context, any) (any, error) {
		return "async-notimeout", nil
	}, ListenerConfig{Name: "async-notimeout", Async: true})

	pending := NewValidationContext(t.TempDir())
	res, err := WaitForResult(ProcessContext(pending), 8*time.Second)
	if err != nil || res == nil {
		t.Fatalf("pending process: %v %v", res, err)
	}

	ClearAllListeners()
	RegisterListener(StatePending, func(stdcontext.Context, any) (any, error) {
		return nil, errFmt("required-boom")
	}, ListenerConfig{Name: "req", Required: true})
	failing := NewValidationContext(t.TempDir())
	res, err = WaitForResult(ProcessContext(failing), 5*time.Second)
	if err != nil || res == nil {
		t.Fatalf("required fail process: %v %v", res, err)
	}

	done := make(chan *ProcessingResult, 1)
	done <- &ProcessingResult{State: StateCompleted}
	close(done)
	if _, err := WaitForResult(done, 0); err != nil {
		t.Fatal(err)
	}

	_ = WithValidationProgress(nil, nil)
	_ = GetValidationProgress(nil)

	_ = IsSystemAccount(SystemAccountID)
	_ = IsSystemAccount("system")
	_ = IsSystemAccount("account:system")
	_ = ActorIDForAttribution(SystemAccountID)

	sec := NewSecurityContext("acc", nil, nil)
	sec.LLMKeys = map[string]string{"openai": "k", "gemini": "g", "qwen": "q"}
	sec.LLMBaseURLs = map[string]string{"openai": "http://o", "gemini": "http://g", "qwen": "http://q"}
	_ = sec.GetLLMAPIKey("openai")
	_ = sec.GetLLMAPIKey("gemini")
	_ = sec.GetLLMAPIKey("qwen")
	_ = sec.GetLLMAPIKey("other")
	_ = sec.GetLLMBaseURL("openai")
	_ = sec.GetLLMBaseURL("gemini")
	_ = sec.GetLLMBaseURL("qwen")
	_ = sec.GetLLMBaseURL("other")
	_ = NewSecurityContext("", nil, nil).Validate()

	badStore := NewStorageContext()
	badStore.MaxPageSize = -1
	badStore.DefaultPageSize = -1
	badStore.MaxGroupSize = -1
	if len(badStore.Validate()) < 3 {
		t.Fatal("storage validate")
	}
	hi := NewPaginationStorageContext(50, 10)
	hi.SetPrecedence(0)
	lo := NewPaginationStorageContext(5, 1)
	lo.SetPrecedence(80)
	_ = hi.Merge(lo)
	_ = lo.Merge(hi)
	_ = hi.Merge(NewSystemLoggingContext())

	cli := NewCliInitializationContext(func(string) string { return "" }, ".")
	cli.ProjectRoot = ""
	_ = cli.Validate()
	_ = cli.Merge(NewSystemLoggingContext())

	ldc := NewLoggingDecisionContext().WithComponent("storage").WithOperation("write")
	_ = ldc.ShouldLog(1, "storage", "write")
	_ = ldc.ShouldLog(0, "storage", "write")
	_ = ldc.ShouldLog(1, "other", "write")
	_ = ldc.ShouldLog(1, "storage", "other")
	ldc.WithVerbose(true)
	_ = ldc.ShouldLog(1, "other", "other")
	agent := NewLoggingDecisionContext().WithLoggingContext(NewLoggingContext(ProfileAIAgent))
	_ = agent.GetDestinations(1, t.TempDir())
	dbg := NewLoggingDecisionContext().WithLoggingContext(NewLoggingContext(ProfileDebug).WithSuppressDebugToStdout(true))
	_ = dbg.GetDestinations(0, t.TempDir())
	nilLog := &LoggingDecisionContext{}
	_ = nilLog.GetLogLevel()
	_ = nilLog.Validate()
	_ = nilLog.GetDestinations(1, t.TempDir())
}

func errFmt(msg string) error {
	return &simpleErr{msg}
}

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
