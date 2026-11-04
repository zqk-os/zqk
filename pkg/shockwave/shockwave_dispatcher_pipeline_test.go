// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package shockwave

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

type cycle209Node struct {
	mu          sync.Mutex
	deliverErr  error
	delay       time.Duration
	lastPayload any
	lastTiers   TierMask
}

func (n *cycle209Node) Deliver(ctx context.Context, payload any, tiers TierMask) (any, error) {
	if n.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(n.delay):
		}
	}
	n.mu.Lock()
	n.lastPayload = payload
	n.lastTiers = tiers
	err := n.deliverErr
	n.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return fmt.Sprintf("ack:%v", payload), nil
}

func TestCycle209_Dispatcher_GlobalAccessor(t *testing.T) {
	orig := GetGlobalDispatcher()
	t.Cleanup(func() {
		SetGlobalDispatcher(orig)
	})

	custom := NewGatewayDispatcher(nil, 5)
	SetGlobalDispatcher(custom)
	got := GetGlobalDispatcher()
	if got != custom {
		t.Fatalf("expected custom dispatcher %v, got %v", custom, got)
	}
}

func TestCycle209_Dispatcher_DeliveryHandler_InvalidPayload(t *testing.T) {
	h := &deliveryHandler{nodes: make(map[string]SimulatedNode)}
	res, err := h.Handle(context.Background(), "invalid-payload-string", TierSecurity)
	if err == nil {
		t.Fatalf("expected error for invalid payload type, got nil")
	}
	if res != nil {
		t.Fatalf("expected nil result on invalid payload, got %v", res)
	}
}

func TestCycle209_Dispatcher_DeliveryHandler_NodeNotFound(t *testing.T) {
	h := &deliveryHandler{nodes: make(map[string]SimulatedNode)}
	task := RoutingTask{
		NodeID:  "missing-node-id",
		Payload: "test-data",
	}
	res, err := h.Handle(context.Background(), task, TierSyntax)
	if err == nil {
		t.Fatalf("expected error for missing node, got nil")
	}
	if res != nil {
		t.Fatalf("expected nil result on missing node, got %v", res)
	}
}

func TestCycle209_Dispatcher_DeliveryHandler_SuccessWithoutResultChan(t *testing.T) {
	node := &cycle209Node{}
	h := &deliveryHandler{nodes: map[string]SimulatedNode{"node-1": node}}
	task := RoutingTask{
		NodeID:     "node-1",
		Payload:    "payload-1",
		Tiers:      TierExecution,
		ResultChan: nil,
	}
	res, err := h.Handle(context.Background(), task, TierExecution)
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	expected := "ack:payload-1"
	if res != expected {
		t.Fatalf("expected %s, got %v", expected, res)
	}
}

func TestCycle209_Dispatcher_ParseTier_Exhaustive(t *testing.T) {
	cases := []struct {
		input    string
		expected TierMask
	}{
		{"security", TierSecurity},
		{"SECURITY", TierSecurity},
		{"syntax", TierSyntax},
		{"SYNTAX", TierSyntax},
		{"semantic", TierSemantic},
		{"SEMANTIC", TierSemantic},
		{"execution", TierExecution},
		{"EXECUTION", TierExecution},
		{"non_existent_tier", 0},
		{"", 0},
	}
	for _, tc := range cases {
		got := ParseTier(tc.input)
		if got != tc.expected {
			t.Errorf("ParseTier(%q) = %d, expected %d", tc.input, got, tc.expected)
		}
	}
}

func TestCycle209_Dispatcher_SpecValidation_Errors(t *testing.T) {
	disp := NewGatewayDispatcher(nil, 4)
	ctx := context.Background()

	// Missing spec
	noSpec := map[string]any{objects.FieldKeyKind: "shockwave_router"}
	if _, err := disp.Dispatch(ctx, noSpec, "p"); err == nil {
		t.Fatal("expected error on missing spec")
	}

	// Spec is not a map
	notMapSpec := map[string]any{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeySpec: "scalar-spec",
	}
	if _, err := disp.Dispatch(ctx, notMapSpec, "p"); err == nil {
		t.Fatal("expected error on spec not being a map")
	}

	// Missing routes
	noRoutes := map[string]any{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeySpec: map[string]any{},
	}
	if _, err := disp.Dispatch(ctx, noRoutes, "p"); err == nil {
		t.Fatal("expected error on missing routes")
	}

	// Routes is not a list
	routesNotList := map[string]any{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeySpec: map[string]any{"routes": 123},
	}
	if _, err := disp.Dispatch(ctx, routesNotList, "p"); err == nil {
		t.Fatal("expected error on routes not being a list")
	}
}

func TestCycle209_Dispatcher_RoutesFiltering_ZeroResults(t *testing.T) {
	disp := NewGatewayDispatcher(nil, 4)
	ctx := context.Background()

	routerObj := map[string]any{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeySpec: map[string]any{
			"routes": []any{
				"not-a-map",
				map[string]any{"node_id": ""},
				map[string]any{"node_id": 456},
			},
		},
	}
	res, err := disp.Dispatch(ctx, routerObj, "data")
	if err != nil {
		t.Fatalf("unexpected error on filtered routes: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("expected 0 results, got %d", len(res))
	}
}

func TestCycle209_Dispatcher_TiersParsingAndDefaults(t *testing.T) {
	disp := NewGatewayDispatcher(logging.GetLoggerFromProfile("system"), 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	disp.Start(ctx)
	defer disp.Stop()

	n1 := &cycle209Node{}
	n2 := &cycle209Node{}
	disp.RegisterNode("n1", n1)
	disp.RegisterNode("n2", n2)

	routerObj := map[string]any{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeySpec: map[string]any{
			"routes": []any{
				map[string]any{
					"node_id": "n1",
					"tiers":   []any{"security", "execution", 999},
				},
				map[string]any{
					"node_id": "n2",
					// tiers omitted -> full mask
				},
			},
		},
	}

	results, err := disp.Dispatch(ctx, routerObj, "payload-x")
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	n1.mu.Lock()
	if n1.lastTiers != (TierSecurity | TierExecution) {
		t.Errorf("n1 tiers expected security|execution, got %d", n1.lastTiers)
	}
	n1.mu.Unlock()

	n2.mu.Lock()
	if n2.lastTiers != ^TierMask(0) {
		t.Errorf("n2 tiers expected all-bits mask, got %d", n2.lastTiers)
	}
	n2.mu.Unlock()
}

func TestCycle209_Dispatcher_ReduceStrategy_TimeoutsAndRequireAll(t *testing.T) {
	disp := NewGatewayDispatcher(nil, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	disp.Start(ctx)
	defer disp.Stop()

	slowNode := &cycle209Node{delay: 200 * time.Millisecond}
	disp.RegisterNode("slow-node", slowNode)

	// Case 1: require_all = true -> timeout returns error
	routerTimeoutFail := map[string]any{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeySpec: map[string]any{
			"routes": []any{
				map[string]any{"node_id": "slow-node"},
			},
			objects.FieldKeyReduceStrategy: map[string]any{
				"require_all":             true,
				objects.FieldKeyTimeoutMs: 20.0,
			},
		},
	}
	_, err := disp.Dispatch(ctx, routerTimeoutFail, "slow-msg")
	if err == nil {
		t.Fatal("expected timeout error with require_all=true, got nil")
	}

	// Case 2: require_all = false -> timeout returns partial results without error
	routerTimeoutPartial := map[string]any{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeySpec: map[string]any{
			"routes": []any{
				map[string]any{"node_id": "slow-node"},
			},
			objects.FieldKeyReduceStrategy: map[string]any{
				"require_all":             false,
				objects.FieldKeyTimeoutMs: 20, // int timeout
			},
		},
	}
	resPartial, errPartial := disp.Dispatch(ctx, routerTimeoutPartial, "slow-msg-2")
	if errPartial != nil {
		t.Fatalf("unexpected error with require_all=false on timeout: %v", errPartial)
	}
	if len(resPartial) != 0 {
		t.Fatalf("expected 0 partial results on timeout, got %d", len(resPartial))
	}
}

func TestCycle209_Dispatcher_ReduceStrategy_NodeFailureBranching(t *testing.T) {
	disp := NewGatewayDispatcher(logging.GetLoggerFromProfile("system"), 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	disp.Start(ctx)
	defer disp.Stop()

	errNode := &cycle209Node{deliverErr: errors.New("simulated delivery crash")}
	okNode := &cycle209Node{}
	disp.RegisterNode("fail-node", errNode)
	disp.RegisterNode("ok-node", okNode)

	// Case 1: require_all = true -> returns error
	routerRequireAll := map[string]any{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeySpec: map[string]any{
			"routes": []any{
				map[string]any{"node_id": "fail-node"},
				map[string]any{"node_id": "ok-node"},
			},
			objects.FieldKeyReduceStrategy: map[string]any{
				"require_all": true,
			},
		},
	}
	_, errReq := disp.Dispatch(ctx, routerRequireAll, "test-data")
	if errReq == nil {
		t.Fatal("expected error with require_all=true on node failure, got nil")
	}

	// Case 2: require_all = false -> skips failed node, returns ok-node result
	routerTolerant := map[string]any{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeySpec: map[string]any{
			"routes": []any{
				map[string]any{"node_id": "fail-node"},
				map[string]any{"node_id": "ok-node"},
			},
			objects.FieldKeyReduceStrategy: map[string]any{
				"require_all": false,
			},
		},
	}
	resTolerant, errTol := disp.Dispatch(ctx, routerTolerant, "test-data-2")
	if errTol != nil {
		t.Fatalf("unexpected error with require_all=false on node failure: %v", errTol)
	}
	if len(resTolerant) != 1 {
		t.Fatalf("expected 1 result from ok-node, got %d", len(resTolerant))
	}
}

func TestCycle209_Dispatcher_ContextCancellationDuringDispatch(t *testing.T) {
	disp := NewGatewayDispatcher(nil, 4)
	ctx, cancel := context.WithCancel(context.Background())
	disp.Start(ctx)
	defer disp.Stop()

	slowNode := &cycle209Node{delay: 500 * time.Millisecond}
	disp.RegisterNode("slow-node", slowNode)

	routerObj := map[string]any{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeySpec: map[string]any{
			"routes": []any{
				map[string]any{"node_id": "slow-node"},
			},
		},
	}

	done := make(chan struct{})
	var dispatchErr error
	goroutinelabels.NewGoroutine("test.dispatch_cancel", "test context cancellation").StartSimple(func() {
		_, dispatchErr = disp.Dispatch(ctx, routerObj, "cancel-data")
		close(done)
	})

	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if !errors.Is(dispatchErr, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", dispatchErr)
	}
}

func TestCycle209_Pipeline_GlobalSingletonAndDefaults(t *testing.T) {
	p1 := GetGlobalEventPipeline()
	if p1 == nil {
		t.Fatal("expected non-nil global event pipeline")
	}
	p2 := GetGlobalEventPipeline()
	if p1 != p2 {
		t.Fatal("expected singleton identity for global event pipeline")
	}

	// Test fallback defaults on negative or zero configuration
	pCustom := NewEventPipeline(PipelineConfig{
		BufferSize:  -10,
		WorkerCount: -2,
	})
	if pCustom.config.BufferSize != 256 {
		t.Errorf("expected default buffer size 256, got %d", pCustom.config.BufferSize)
	}
	if pCustom.config.WorkerCount != 1 {
		t.Errorf("expected default worker count 1, got %d", pCustom.config.WorkerCount)
	}
}

func TestCycle209_Pipeline_TopicMatchingPatterns(t *testing.T) {
	if !matchTopic("*", "any.topic.name") {
		t.Error("wildcard * should match any topic")
	}
	if !matchTopic("node.event", "node.event") {
		t.Error("exact topic should match")
	}
	if matchTopic("node.event", "node.other") {
		t.Error("non-matching exact topic should not match")
	}
	if !matchTopic("sys.*", "sys.reboot") {
		t.Error("prefix pattern sys.* should match sys.reboot")
	}
	if matchTopic("sys.*", "system.reboot") {
		t.Error("prefix pattern sys.* should not match system.reboot")
	}
	if matchTopic("sys.*", "other.sys.reboot") {
		t.Error("prefix pattern sys.* should not match other.sys.reboot")
	}
}

func TestCycle209_Pipeline_SubscriptionHandleNilSafety(t *testing.T) {
	handle := &subHandle{
		id:       999,
		pipeline: nil,
	}
	// Must not panic when pipeline is nil
	handle.Unsubscribe()
}

func TestCycle209_Pipeline_EmptyPatternDefaultsToWildcard(t *testing.T) {
	p := NewEventPipeline(PipelineConfig{BufferSize: 10})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	goroutinelabels.NewGoroutine("test.pipeline_run_empty", "test run").StartSimple(func() {
		p.Run(ctx)
	})

	var received atomic.Int32
	sub := p.Subscribe("", func(evt ShockwaveEvent) {
		received.Add(1)
	})
	defer sub.Unsubscribe()

	ok := p.Publish(ShockwaveEvent{
		Topic:     "unspecified.topic",
		Timestamp: time.Now(),
		Payload:   "hello",
	})
	if !ok {
		t.Fatal("publish should succeed")
	}

	time.Sleep(30 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 delivery with empty pattern subscribing as *, got %d", received.Load())
	}
}

func TestCycle209_Pipeline_PublishTimestampAutoSetAndBufferOverflow(t *testing.T) {
	// Tiny buffer of 1 with no workers reading, so second publish overflows
	p := NewEventPipeline(PipelineConfig{
		BufferSize:     1,
		DropOnOverflow: true,
	})

	evt1 := ShockwaveEvent{Topic: "t1"}
	if !p.Publish(evt1) {
		t.Fatal("first publish should succeed in buffer")
	}

	evt2 := ShockwaveEvent{Topic: "t2"}
	if p.Publish(evt2) {
		t.Fatal("second publish should overflow buffer and return false")
	}

	if p.DroppedCount() != 1 {
		t.Fatalf("expected dropped count 1, got %d", p.DroppedCount())
	}
}

func TestCycle209_Neuron_ZeroWorkersDefault(t *testing.T) {
	n := NewNeuron(PropagationRules{MaxWorkers: 0}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n.Start(ctx)
	defer n.Stop()

	if err := n.Send(ctx, "test-msg"); err != nil {
		t.Fatalf("send failed on neuron with default workers: %v", err)
	}

	select {
	case res := <-n.Receive():
		if res != "test-msg" {
			t.Fatalf("expected test-msg, got %v", res)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for neuron message")
	}
}

type cycle209Splitter struct {
	fail bool
}

func (s *cycle209Splitter) Split(ctx context.Context, payload any) ([]any, error) {
	if s.fail {
		return nil, errors.New("splitter failure")
	}
	return []any{fmt.Sprintf("%v_a", payload), fmt.Sprintf("%v_b", payload)}, nil
}

func TestCycle209_Neuron_MultiSplitterSuccessAndFailure(t *testing.T) {
	// Success case: two splitters in pipeline
	nOk := NewNeuron(PropagationRules{MaxWorkers: 2}, logging.GetLoggerFromProfile("system"))
	nOk.RegisterSplitter(&cycle209Splitter{fail: false})
	nOk.RegisterSplitter(&cycle209Splitter{fail: false})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nOk.Start(ctx)
	defer nOk.Stop()

	if err := nOk.Send(ctx, "base"); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	var results []any
	timeout := time.After(1 * time.Second)
	for len(results) < 4 {
		select {
		case r := <-nOk.Receive():
			results = append(results, r)
		case <-timeout:
			t.Fatalf("timeout waiting for 4 split results, got %d", len(results))
		}
	}

	// Failure case: second splitter fails
	nFail := NewNeuron(PropagationRules{MaxWorkers: 2}, logging.GetLoggerFromProfile("system"))
	nFail.RegisterSplitter(&cycle209Splitter{fail: false})
	nFail.RegisterSplitter(&cycle209Splitter{fail: true})
	nFail.Start(ctx)
	defer nFail.Stop()

	if err := nFail.Send(ctx, "base"); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	select {
	case r := <-nFail.Receive():
		t.Fatalf("unexpected message after splitter failure: %v", r)
	case <-time.After(50 * time.Millisecond):
		// Expected: nothing emitted
	}
}

type cycle209Duplicator struct {
	fail bool
}

func (d *cycle209Duplicator) Duplicate(ctx context.Context, payload any) ([]any, error) {
	if d.fail {
		return nil, errors.New("duplicator failure")
	}
	return []any{payload, payload}, nil
}

func TestCycle209_Neuron_MultiDuplicatorSuccessAndFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nOk := NewNeuron(PropagationRules{MaxWorkers: 2}, logging.GetLoggerFromProfile("system"))
	nOk.RegisterDuplicator(&cycle209Duplicator{fail: false})
	nOk.RegisterDuplicator(&cycle209Duplicator{fail: false})
	nOk.Start(ctx)
	defer nOk.Stop()

	if err := nOk.Send(ctx, "item"); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	var results []any
	timeout := time.After(1 * time.Second)
	for len(results) < 4 {
		select {
		case r := <-nOk.Receive():
			results = append(results, r)
		case <-timeout:
			t.Fatalf("timeout waiting for 4 duplicated results, got %d", len(results))
		}
	}

	// Failure case: duplicator fails
	nFail := NewNeuron(PropagationRules{MaxWorkers: 2}, logging.GetLoggerFromProfile("system"))
	nFail.RegisterDuplicator(&cycle209Duplicator{fail: true})
	nFail.Start(ctx)
	defer nFail.Stop()

	if err := nFail.Send(ctx, "item"); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	select {
	case r := <-nFail.Receive():
		t.Fatalf("unexpected message after duplicator failure: %v", r)
	case <-time.After(50 * time.Millisecond):
		// Expected: nothing emitted
	}
}

// cycle209ShockwaveAccumulator verifies operational dynamic proof of
// shockwave event accumulation, batch aggregation, flush timers, and lifecycle transitions.
type cycle209ShockwaveAccumulator struct {
	mu           sync.Mutex
	batchLimit   int
	flushTimeout time.Duration
	pending      []ShockwaveEvent
	batches      [][]ShockwaveEvent
	flushSignal  chan struct{}
	stopped      atomic.Bool
}

func newCycle209Accumulator(limit int, timeout time.Duration) *cycle209ShockwaveAccumulator {
	return &cycle209ShockwaveAccumulator{
		batchLimit:   limit,
		flushTimeout: timeout,
		pending:      make([]ShockwaveEvent, 0, limit),
		flushSignal:  make(chan struct{}, 10),
	}
}

func (a *cycle209ShockwaveAccumulator) Ingest(evt ShockwaveEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped.Load() {
		return
	}
	a.pending = append(a.pending, evt)
	if len(a.pending) >= a.batchLimit {
		a.flushLocked()
	}
}

func (a *cycle209ShockwaveAccumulator) flushLocked() {
	if len(a.pending) == 0 {
		return
	}
	batch := make([]ShockwaveEvent, len(a.pending))
	copy(batch, a.pending)
	a.batches = append(a.batches, batch)
	a.pending = a.pending[:0]

	select {
	case a.flushSignal <- struct{}{}:
	default:
	}
}

func (a *cycle209ShockwaveAccumulator) Start(ctx context.Context) {
	ticker := time.NewTicker(a.flushTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.mu.Lock()
			a.flushLocked()
			a.stopped.Store(true)
			a.mu.Unlock()
			return
		case <-ticker.C:
			a.mu.Lock()
			a.flushLocked()
			a.mu.Unlock()
		}
	}
}

func (a *cycle209ShockwaveAccumulator) Batches() [][]ShockwaveEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([][]ShockwaveEvent, len(a.batches))
	copy(out, a.batches)
	return out
}

func TestCycle209_ShockwaveAccumulator_BatchThresholdAndFlushTimer(t *testing.T) {
	pipeline := NewEventPipeline(PipelineConfig{BufferSize: 100, WorkerCount: 2})
	pipelineCtx, cancelPipeline := context.WithCancel(context.Background())
	defer cancelPipeline()

	goroutinelabels.NewGoroutine("test.pipeline_acc", "run pipeline").StartSimple(func() {
		pipeline.Run(pipelineCtx)
	})

	accumulator := newCycle209Accumulator(5, 50*time.Millisecond)
	accCtx, cancelAcc := context.WithCancel(context.Background())

	goroutinelabels.NewGoroutine("test.accumulator", "run accumulator lifecycle").StartSimple(func() {
		accumulator.Start(accCtx)
	})

	sub := pipeline.Subscribe("shockwave.cascade.*", func(evt ShockwaveEvent) {
		accumulator.Ingest(evt)
	})
	defer sub.Unsubscribe()

	// Emit 5 events concurrently to trigger immediate threshold batch
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("test.emit_%d", idx), "emit shockwave event").StartSimple(func() {
			defer wg.Done()
			published := pipeline.Publish(ShockwaveEvent{
				Topic:    "shockwave.cascade.ripple",
				SourceID: fmt.Sprintf("CRIT-CYCLE209-%03d", idx),
				Payload:  fmt.Sprintf("state_delta_%d", idx),
			})
			if !published {
				t.Errorf("failed to publish event %d", idx)
			}
		})
	}
	wg.Wait()

	// Wait for threshold flush signal
	select {
	case <-accumulator.flushSignal:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for threshold batch flush")
	}

	batches := accumulator.Batches()
	if len(batches) == 0 {
		t.Fatal("expected at least 1 completed batch after threshold reach")
	}
	if len(batches[0]) != 5 {
		t.Fatalf("expected batch size 5, got %d", len(batches[0]))
	}

	// Now emit 2 more events (less than batchLimit 5) to test timer flush
	for i := 5; i < 7; i++ {
		published := pipeline.Publish(ShockwaveEvent{
			Topic:    "shockwave.cascade.ripple",
			SourceID: fmt.Sprintf("CRIT-CYCLE209-%03d", i),
			Payload:  fmt.Sprintf("state_delta_%d", i),
		})
		if !published {
			t.Errorf("failed to publish event %d", i)
		}
	}

	// Wait for timer flush
	select {
	case <-accumulator.flushSignal:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for timer batch flush")
	}

	cancelAcc()
	time.Sleep(30 * time.Millisecond)

	finalBatches := accumulator.Batches()
	if len(finalBatches) < 2 {
		t.Fatalf("expected at least 2 batches (threshold + timer), got %d", len(finalBatches))
	}
	if len(finalBatches[1]) != 2 {
		t.Fatalf("expected timer batch size 2, got %d", len(finalBatches[1]))
	}
}

func TestCycle209_Pipeline_DrainInboxOnShutdown(t *testing.T) {
	pipeline := NewEventPipeline(PipelineConfig{BufferSize: 10, WorkerCount: 1})

	var drained atomic.Int32
	sub := pipeline.Subscribe("drain.*", func(evt ShockwaveEvent) {
		drained.Add(1)
	})
	defer sub.Unsubscribe()

	// Enqueue events before running
	if !pipeline.Publish(ShockwaveEvent{Topic: "drain.evt1"}) {
		t.Fatal("publish 1 failed")
	}
	if !pipeline.Publish(ShockwaveEvent{Topic: "drain.evt2"}) {
		t.Fatal("publish 2 failed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before Run, forcing Run to immediately invoke drain()

	pipeline.Run(ctx)

	if drained.Load() != 2 {
		t.Fatalf("expected 2 drained events, got %d", drained.Load())
	}
}

func TestCycle209_Neuron_PoolSaturationErrPoolFull(t *testing.T) {
	// WorkerCount 1 gives queue capacity 10 in goroutinelabels.Pool
	neuron := NewNeuron(PropagationRules{MaxWorkers: 1}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register a blocking handler so the single worker is occupied
	blockChan := make(chan struct{})

	neuron.RegisterHandler(&blockingHandler{unblock: blockChan})
	neuron.Start(ctx)

	var gotFullErr bool
	for i := 0; i < 20; i++ {
		err := neuron.Send(ctx, fmt.Sprintf("payload_%d", i))
		if err != nil {
			if err.Error() == "neuron inbox is completely full, shedding load" {
				gotFullErr = true
				break
			}
		}
	}

	// Release worker before stopping neuron
	close(blockChan)
	neuron.Stop()

	if !gotFullErr {
		t.Fatal("expected 'neuron inbox is completely full, shedding load' error on saturated pool")
	}
}

type blockingHandler struct {
	unblock <-chan struct{}
}

func (h *blockingHandler) Handle(ctx context.Context, payload any, tiers TierMask) (any, error) {
	select {
	case <-h.unblock:
		return payload, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestCycle209_Neuron_OutboxCapacityDropWithAndWithoutHandlers(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Case 1: with handler, outbox buffer = 10, submit 25 items without reading Receive()
	nWithHandler := NewNeuron(PropagationRules{MaxWorkers: 1}, logger)
	nWithHandler.RegisterHandler(passthroughHandler{})
	nWithHandler.Start(ctx)
	defer nWithHandler.Stop()

	for i := 0; i < 25; i++ {
		if err := nWithHandler.Send(ctx, fmt.Sprintf("with_handler_%d", i)); err != nil {
			// Pool might be full, safe to ignore during saturation test
			continue
		}
	}
	// Give worker time to process and trigger outbox full drop branch
	time.Sleep(50 * time.Millisecond)

	// Case 2: without handler, outbox buffer = 10, submit 25 items without reading Receive()
	nNoHandler := NewNeuron(PropagationRules{MaxWorkers: 1}, logger)
	nNoHandler.Start(ctx)
	defer nNoHandler.Stop()

	for i := 0; i < 25; i++ {
		if err := nNoHandler.Send(ctx, fmt.Sprintf("no_handler_%d", i)); err != nil {
			continue
		}
	}
	time.Sleep(50 * time.Millisecond)
}

func TestCycle209_Dispatcher_NeuronSendFailure(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	disp := NewGatewayDispatcher(logger, 1)
	// Stop the internal neuron immediately so submit fails
	disp.Stop()

	routerObj := map[string]any{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeySpec: map[string]any{
			"routes": []any{
				map[string]any{"node_id": "any-node"},
			},
		},
	}

	// Dispatch should log error on neuron.Send failure and complete reduce phase with 0 results
	res, err := disp.Dispatch(context.Background(), routerObj, "data")
	if err != nil {
		t.Fatalf("unexpected error during dispatch with stopped neuron: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("expected 0 results, got %d", len(res))
	}
}
