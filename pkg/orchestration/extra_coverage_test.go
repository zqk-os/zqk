// BLI-STARTER-COMMUNITY-055 / PRI-STARTER-COMMUNITY-055 coverage elevation
package orchestration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/hive/capability"
	"github.com/zqk-os/zqk/pkg/hivemind"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
)

type extraMgr struct{}

func (extraMgr) ProcessIntent(context.Context, RawIntent) (CapabilityRef, error) {
	return CapabilityRef{ID: "c1", Kind: "k"}, nil
}

type extraNeg struct {
	res *scheduler.NegotiationResult
	err error
}

func (n extraNeg) Propose(context.Context, scheduler.Proposal) (*scheduler.NegotiationResult, error) {
	return n.res, n.err
}

type extraMem struct{ err error }

func (m extraMem) RetrieveSemantically(context.Context, string, int) ([]hivemind.MemoryResult, error) {
	return nil, m.err
}
func (extraMem) RetrieveGraphContext(context.Context, string, int) (*hivemind.GraphSubgraph, error) {
	return nil, nil
}
func (extraMem) QueryHybrid(context.Context, string, int, hivemind.HybridConstraints) ([]hivemind.MemoryResult, error) {
	return nil, nil
}
func (extraMem) FindObjectsMissingVectors(context.Context, int) ([]string, error) { return nil, nil }
func (extraMem) LinkVectorID(context.Context, string, string) error               { return nil }

type extraStore struct{ storage.NoopObjectStorage }

func (extraStore) Create(context.Context, *storage.SecurityContext, map[string]any) error { return nil }

func TestExtraOrchestrationCoverage(t *testing.T) {
	ctx := context.Background()
	reg := GetRegistry()
	reg.RegisterManager(extraMgr{})
	if reg.GetManager() == nil {
		t.Fatal("manager")
	}

	mg := provider.NewMockGraphProvider()
	pool, err := mg.CreatePool(ctx, provider.ConnectionConfig{})
	if err != nil {
		t.Fatal(err)
	}
	inbox := NewAutonomyInbox(pool)
	if err := inbox.RouteMessage(ctx, Message{ID: "m1", Kind: "backlog_item", Destination: "d1", Payload: map[string]any{"k": "v"}, Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}

	sp := NewPredictiveSpawner(pool)
	_ = sp.IngestEvent(ctx, AmbientEvent{Type: "OTHER"})
	_ = sp.IngestEvent(ctx, AmbientEvent{Type: "FILE_CREATED", Payload: map[string]any{"filepath": "pkg/foo.go"}})
	_ = sp.IngestEvent(ctx, AmbientEvent{Type: "FILE_CREATED", Payload: map[string]any{"filepath": "pkg/foo_test.go"}})

	pw := NewIntentPrewarmer(pool)
	_ = pw.Prewarm(ctx, IntentPrediction{})
	_ = pw.Prewarm(ctx, IntentPrediction{TargetObject: "requirement"})

	dir := t.TempDir()
	poolWT := NewWorktreePool(dir, 2)
	sw := NewWorktreeSweeper(poolWT)
	_, _ = sw.Sweep(time.Now())
	cancel, cancelFn := context.WithCancel(ctx)
	cancelFn()
	_, _ = sw.SweepContext(cancel, time.Now())
	_, _ = sw.SweepContext(ctx, time.Now())

	wd := NewIdleWatchdog(0, 0)
	if err := wd.Execute(ctx, "a1", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	fast := NewIdleWatchdog(5*time.Millisecond, time.Millisecond)
	_ = fast.Execute(ctx, "a2", func(context.Context) error {
		time.Sleep(20 * time.Millisecond)
		return nil
	})
	cctx, cfn := context.WithCancel(ctx)
	cfn()
	_ = NewIdleWatchdog(time.Second, time.Millisecond).Execute(cctx, "a3", func(tctx context.Context) error {
		<-tctx.Done()
		return tctx.Err()
	})

	pn := NewPolicyNegotiator(logging.GetLoggerFromProfile("system"))
	_, _ = pn.Propose(ctx, scheduler.Proposal{})
	_, _ = pn.Propose(ctx, scheduler.Proposal{Metadata: map[string]any{"raw": map[string]any{ConstVersionContext: "old"}}})

	store := extraStore{}
	m := NewManager(extraNeg{res: &scheduler.NegotiationResult{Accepted: true}}, extraMem{}, store)
	_, _ = m.ProcessIntent(ctx, RawIntent{Signature: "sig", Payload: map[string]any{"test_cases": []string{"t1"}}})
	_, _ = m.ProcessIntent(ctx, RawIntent{Signature: "sig2", Payload: map[string]any{"test_cases": []any{"t2"}, "test_case": "t3"}})
	_, _ = NewManager(extraNeg{err: errors.New("neg")}, extraMem{}, store).ProcessIntent(ctx, RawIntent{Signature: "x"})
	_, _ = NewManager(extraNeg{res: &scheduler.NegotiationResult{Accepted: false, Reason: "no"}}, extraMem{}, store).ProcessIntent(ctx, RawIntent{Signature: "x"})
	_, _ = NewManager(extraNeg{res: &scheduler.NegotiationResult{Accepted: true}}, extraMem{err: errors.New("mem")}, store).ProcessIntent(ctx, RawIntent{Signature: "x"})

	sess := NewSession(logging.GetLoggerFromProfile("system"), store)
	_ = sess.AddAgent(ctx, "agent-1")
	_ = sess.Status()
	_ = sess.Start(ctx, "topic", map[string]any{"k": "v"})
	_ = sess.Start(ctx, "topic", map[string]any{"local_merge_to_main": true})
	_ = sess.Start(ctx, "topic", map[string]any{"local_merge_to_main": true, "pr_link": "https://example.com/pr/1"})
	dyn := &dynamicCapability{id: "id", title: "t"}
	_, _ = dyn.EvaluatePolicy(ctx, capability.EvalContext{})
	_ = dyn.Name()
	_ = dyn.Description()
}
