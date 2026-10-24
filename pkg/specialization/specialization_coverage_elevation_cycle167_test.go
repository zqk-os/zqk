// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package specialization

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type mockSpineWithErr struct {
	mu           sync.Mutex
	subscribeErr map[string]error
	publishErr   error
	published    []infrastructure.Event
	handlers     map[string]infrastructure.Handler
}

func newMockSpineWithErr() *mockSpineWithErr {
	return &mockSpineWithErr{
		subscribeErr: make(map[string]error),
		handlers:     make(map[string]infrastructure.Handler),
	}
}

func (m *mockSpineWithErr) Publish(_ context.Context, event infrastructure.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.publishErr != nil {
		return m.publishErr
	}
	m.published = append(m.published, event)
	return nil
}

func (m *mockSpineWithErr) Subscribe(_ context.Context, kind string, handler infrastructure.Handler) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.subscribeErr[kind]; ok && err != nil {
		return err
	}
	m.handlers[kind] = handler
	return nil
}

func (m *mockSpineWithErr) Replay(_ context.Context, _ int64, _ infrastructure.Handler) error {
	return nil
}

func (m *mockSpineWithErr) Close() error {
	return nil
}

type mockFailingHandler struct {
	initErr  error
	startErr error
	stopErr  error
	stopped  bool
}

func (m *mockFailingHandler) Initialize(_ context.Context, _ storage.ObjectStorageProvider, _ infrastructure.SpinalSpine) error {
	return m.initErr
}

func (m *mockFailingHandler) Start(_ context.Context) error {
	return m.startErr
}

func (m *mockFailingHandler) Stop() error {
	m.stopped = true
	return m.stopErr
}

type configurableMockStorage struct {
	storage.ObjectStorageProvider
	updateErr error
	createErr error
	created   []map[string]any
	updated   []map[string]any
}

func (m *configurableMockStorage) Create(_ context.Context, _ *storage.SecurityContext, obj map[string]any) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.created = append(m.created, obj)
	return nil
}

func (m *configurableMockStorage) Update(_ context.Context, _ *storage.SecurityContext, _ string, obj map[string]any) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updated = append(m.updated, obj)
	return nil
}

func TestGetHandlersForTier(t *testing.T) {
	neuronHandlers := GetHandlersForTier(TierNeuron)
	if len(neuronHandlers) != 1 {
		t.Fatalf("expected 1 neuron handler, got %d", len(neuronHandlers))
	}
	if _, ok := neuronHandlers[0].(*NeuronHandler); !ok {
		t.Errorf("expected *NeuronHandler, got %T", neuronHandlers[0])
	}

	muscleHandlers := GetHandlersForTier(TierMuscle)
	if len(muscleHandlers) != 1 {
		t.Fatalf("expected 1 muscle handler, got %d", len(muscleHandlers))
	}
	if _, ok := muscleHandlers[0].(*MuscleHandler); !ok {
		t.Errorf("expected *MuscleHandler, got %T", muscleHandlers[0])
	}

	heartHandlers := GetHandlersForTier(TierHeart)
	if len(heartHandlers) != 1 {
		t.Fatalf("expected 1 heart handler, got %d", len(heartHandlers))
	}
	if _, ok := heartHandlers[0].(*HeartHandler); !ok {
		t.Errorf("expected *HeartHandler, got %T", heartHandlers[0])
	}

	allHandlers := GetHandlersForTier(TierAll)
	if len(allHandlers) != 3 {
		t.Fatalf("expected 3 handlers for TierAll, got %d", len(allHandlers))
	}

	lungHandlers := GetHandlersForTier(TierLung)
	if lungHandlers != nil {
		t.Errorf("expected nil handlers for TierLung, got %v", lungHandlers)
	}

	unknownHandlers := GetHandlersForTier(Tier("unknown"))
	if unknownHandlers != nil {
		t.Errorf("expected nil handlers for unknown tier, got %v", unknownHandlers)
	}
}

func TestRegistryLifecycle(t *testing.T) {
	ctx := context.Background()
	spine := newMockSpineWithErr()
	store := &configurableMockStorage{}

	t.Run("EngageAndDisengageSuccess", func(t *testing.T) {
		h1 := &mockFailingHandler{}
		h2 := &mockFailingHandler{}
		reg := NewRegistry()
		reg.Add(h1)
		reg.Add(h2)

		err := reg.EngageAll(ctx, store, spine)
		if err != nil {
			t.Fatalf("expected EngageAll to succeed, got %v", err)
		}

		reg.DisengageAll()
		if !h1.stopped || !h2.stopped {
			t.Errorf("expected all handlers stopped, got h1=%v, h2=%v", h1.stopped, h2.stopped)
		}
	})

	t.Run("EngageInitError", func(t *testing.T) {
		expectedErr := errors.New("init failed")
		h := &mockFailingHandler{initErr: expectedErr}
		reg := NewRegistry()
		reg.Add(h)

		err := reg.EngageAll(ctx, store, spine)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("EngageStartError", func(t *testing.T) {
		expectedErr := errors.New("start failed")
		h := &mockFailingHandler{startErr: expectedErr}
		reg := NewRegistry()
		reg.Add(h)

		err := reg.EngageAll(ctx, store, spine)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})
}

func TestShieldedHandlerLifecycle(t *testing.T) {
	ctx := context.Background()
	spine := newMockSpineWithErr()
	inner := &mockFailingHandler{}
	shielded := NewShieldedHandler(inner)

	if events := shielded.GetShadowEvents(); events != nil {
		t.Errorf("expected nil shadow events before init, got %v", events)
	}

	err := shielded.Initialize(ctx, &configurableMockStorage{}, spine)
	if err != nil {
		t.Fatalf("failed to initialize shielded handler: %v", err)
	}

	err = shielded.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start shielded handler: %v", err)
	}

	err = shielded.Stop()
	if err != nil {
		t.Fatalf("failed to stop shielded handler: %v", err)
	}

	if !inner.stopped {
		t.Errorf("expected inner handler to be stopped")
	}
}

func TestHeartHandlerOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("StartFailOnAuditEvent", func(t *testing.T) {
		spine := newMockSpineWithErr()
		expectedErr := errors.New("audit subscribe fail")
		spine.subscribeErr[objects.KindAuditEvent] = expectedErr

		hh := &HeartHandler{}
		err := hh.Initialize(ctx, &configurableMockStorage{}, spine)
		if err != nil {
			t.Fatalf("unexpected init err: %v", err)
		}
		err = hh.Start(ctx)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("StartFailOnMetrics", func(t *testing.T) {
		spine := newMockSpineWithErr()
		expectedErr := errors.New("metrics subscribe fail")
		spine.subscribeErr["metrics"] = expectedErr

		hh := &HeartHandler{}
		err := hh.Initialize(ctx, &configurableMockStorage{}, spine)
		if err != nil {
			t.Fatalf("unexpected init err: %v", err)
		}
		err = hh.Start(ctx)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("MetricEventAndStop", func(t *testing.T) {
		hh := &HeartHandler{}
		err := hh.handleMetricEvent(ctx, infrastructure.Event{Kind: "metrics"})
		if err != nil {
			t.Fatalf("unexpected metric handle err: %v", err)
		}
		err = hh.Stop()
		if err != nil {
			t.Fatalf("unexpected stop err: %v", err)
		}
	})
}

func TestHeartHandlerVitalityReporting(t *testing.T) {
	ctx := context.Background()

	t.Run("ThrottlingCheck", func(t *testing.T) {
		store := &configurableMockStorage{}
		hh := &HeartHandler{
			store:      store,
			lastUpdate: time.Now(),
		}
		err := hh.updateVitalityReport(ctx)
		if err != nil {
			t.Fatalf("expected nil from throttled update, got %v", err)
		}
		if len(store.updated) > 0 || len(store.created) > 0 {
			t.Errorf("expected no store mutations when throttled")
		}
	})

	t.Run("UpdateFailsCreateSucceeds", func(t *testing.T) {
		store := &configurableMockStorage{
			updateErr: errors.New("not found"),
		}
		hh := &HeartHandler{
			store: store,
		}
		err := hh.updateVitalityReport(ctx)
		if err != nil {
			t.Fatalf("expected fallback to Create to succeed, got %v", err)
		}
		if len(store.created) != 1 {
			t.Errorf("expected 1 created document, got %d", len(store.created))
		}
	})

	t.Run("UpdateAndCreateBothFail", func(t *testing.T) {
		store := &configurableMockStorage{
			updateErr: errors.New("update fail"),
			createErr: errors.New("create fail"),
		}
		hh := &HeartHandler{
			store: store,
		}
		err := hh.updateVitalityReport(ctx)
		if err == nil || err.Error() != "create fail" {
			t.Fatalf("expected create fail error, got %v", err)
		}
	})

	t.Run("AuditEventHandling", func(t *testing.T) {
		store := &configurableMockStorage{}
		hh := &HeartHandler{store: store}

		err := hh.handleAuditEvent(ctx, infrastructure.Event{Op: "convergence_success"})
		if err != nil {
			t.Fatalf("expected audit event success, got %v", err)
		}

		hh.lastUpdate = time.Time{}
		err = hh.handleAuditEvent(ctx, infrastructure.Event{Op: "drift_detected"})
		if err != nil {
			t.Fatalf("expected audit event drift, got %v", err)
		}

		hh.lastUpdate = time.Time{}
		err = hh.handleAuditEvent(ctx, infrastructure.Event{Op: "other_op"})
		if err != nil {
			t.Fatalf("expected audit event other op, got %v", err)
		}
	})
}

func TestHeartHandlerCalculatePCSBounds(t *testing.T) {
	hh := &HeartHandler{
		successRate:    10.0,
		driftFrequency: 0.0,
	}
	scoreMax := hh.calculatePCS()
	if scoreMax != 100 {
		t.Errorf("expected score capped at 100, got %d", scoreMax)
	}

	hhLow := &HeartHandler{
		successRate:    -10.0,
		driftFrequency: 10.0,
	}
	scoreMin := hhLow.calculatePCS()
	if scoreMin != 0 {
		t.Errorf("expected score floored at 0, got %d", scoreMin)
	}
}

func TestHeartHandlerCalculatePCSGoroutineExhaustion(t *testing.T) {
	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	ceiling := concurrency.DefaultGoroutineCeiling
	target := ceiling - int(float64(ceiling)*0.05) // leave < 5% budget

	for runtime.NumGoroutine() < target {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_pcs_exhaustion_waiter", "holds goroutine to simulate compute budget pressure").StartSimple(func() {
			defer wg.Done()
			<-stopCh
		})
	}

	hh := &HeartHandler{
		successRate:    1.0,
		driftFrequency: 0.0,
	}
	score := hh.calculatePCS()

	close(stopCh)
	wg.Wait()

	if score > 49 {
		t.Errorf("expected PCS to drop <= 49 during goroutine exhaustion, got %d", score)
	}
}

func TestNeuronHandlerOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("StartSubscribeFailure", func(t *testing.T) {
		spine := newMockSpineWithErr()
		expectedErr := errors.New("neuron subscribe failed")
		spine.subscribeErr[objects.KindConvergenceSession] = expectedErr

		nh := &NeuronHandler{}
		err := nh.Initialize(ctx, &configurableMockStorage{}, spine)
		if err != nil {
			t.Fatalf("unexpected init err: %v", err)
		}
		err = nh.Start(ctx)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("HandleConvergenceUpdateScenarios", func(t *testing.T) {
		spine := newMockSpineWithErr()
		nh := &NeuronHandler{}
		err := nh.Initialize(ctx, &configurableMockStorage{}, spine)
		if err != nil {
			t.Fatalf("unexpected init err: %v", err)
		}

		err = nh.handleConvergenceUpdate(ctx, infrastructure.Event{
			Payload: map[string]any{
				objects.FieldKeyStatus:          "paused",
				objects.FieldKeyDeltaAssessment: "trending_away",
			},
		})
		if err != nil {
			t.Fatalf("unexpected err on paused session: %v", err)
		}
		if len(spine.published) != 0 {
			t.Errorf("expected 0 published events, got %d", len(spine.published))
		}

		err = nh.handleConvergenceUpdate(ctx, infrastructure.Event{
			Payload: map[string]any{
				objects.FieldKeyStatus:          "active",
				objects.FieldKeyDeltaAssessment: "converged",
			},
		})
		if err != nil {
			t.Fatalf("unexpected err on converged session: %v", err)
		}
		if len(spine.published) != 0 {
			t.Errorf("expected 0 published events, got %d", len(spine.published))
		}

		err = nh.Stop()
		if err != nil {
			t.Fatalf("unexpected stop err: %v", err)
		}
	})
}

func TestMuscleHandlerOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("StartSubscribeFailure", func(t *testing.T) {
		spine := newMockSpineWithErr()
		expectedErr := errors.New("muscle subscribe failed")
		spine.subscribeErr[objects.KindBacklogItem] = expectedErr

		mh := &MuscleHandler{}
		err := mh.Initialize(ctx, &configurableMockStorage{}, spine)
		if err != nil {
			t.Fatalf("unexpected init err: %v", err)
		}
		err = mh.Start(ctx)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("HandleBacklogProposalScenarios", func(t *testing.T) {
		spine := newMockSpineWithErr()
		mh := &MuscleHandler{}
		err := mh.Initialize(ctx, &configurableMockStorage{}, spine)
		if err != nil {
			t.Fatalf("unexpected init err: %v", err)
		}

		err = mh.handleBacklogProposal(ctx, infrastructure.Event{
			Op: "update",
			Payload: map[string]any{
				objects.FieldKeyTitle:        "Task 1",
				objects.FieldKeyPriorityTier: "P1",
			},
		})
		if err != nil {
			t.Fatalf("unexpected err on non-propose: %v", err)
		}
		if len(spine.published) != 0 {
			t.Errorf("expected 0 published events for non-propose op, got %d", len(spine.published))
		}

		err = mh.handleBacklogProposal(ctx, infrastructure.Event{
			Op: "propose",
			Payload: map[string]any{
				objects.FieldKeyTitle:        "Task 2",
				objects.FieldKeyPriorityTier: "P2",
			},
		})
		if err != nil {
			t.Fatalf("unexpected err on P2 propose: %v", err)
		}
		if len(spine.published) != 0 {
			t.Errorf("expected 0 published events for P2 proposal, got %d", len(spine.published))
		}

		err = mh.Stop()
		if err != nil {
			t.Fatalf("unexpected stop err: %v", err)
		}
	})
}

func TestTierTransitionsAndMatrix(t *testing.T) {
	envVar := zqkenv.SpecializationTier()

	testTiers := []struct {
		envVal   string
		defVal   string
		expected Tier
	}{
		{"  NEURON  ", "", TierNeuron},
		{"MUSCLE", "", TierMuscle},
		{"heart", "", TierHeart},
		{"LUNG", "", TierLung},
		{"ALL", "", TierAll},
		{"unknown_tier", "", TierAll},
		{"", "heart", TierHeart},
		{"", "unknown", TierAll},
		{"", "", TierAll},
	}

	for _, tt := range testTiers {
		t.Setenv(envVar.Name(), tt.envVal)
		DefaultTier = tt.defVal
		if got := GetCurrentTier(); got != tt.expected {
			t.Errorf("GetCurrentTier(env=%q, def=%q) = %v, want %v", tt.envVal, tt.defVal, got, tt.expected)
		}
	}

	components := []string{
		"reconciler", "assessor", "inference",
		"scheduler", "importer", "docman",
		"metrics", "health",
		"mesh", "federation",
		"shared-cache",
	}

	for _, c := range components {
		if !TierAll.ShouldRun(c) {
			t.Errorf("TierAll should run %s", c)
		}
	}

	if !TierNeuron.ShouldRun("assessor") || !TierNeuron.ShouldRun("inference") {
		t.Errorf("TierNeuron should run assessor and inference")
	}
	if !TierMuscle.ShouldRun("importer") || !TierMuscle.ShouldRun("docman") {
		t.Errorf("TierMuscle should run importer and docman")
	}
	if !TierHeart.ShouldRun("health") {
		t.Errorf("TierHeart should run health")
	}
	if !TierLung.ShouldRun("federation") {
		t.Errorf("TierLung should run federation")
	}

	if ModeNormal != "normal" || ModeShielded != "shielded" {
		t.Errorf("unexpected Mode constants: normal=%q, shielded=%q", ModeNormal, ModeShielded)
	}
}

func TestSpecializationConcurrency(t *testing.T) {
	ctx := context.Background()
	spine := newMockSpineWithErr()
	store := &configurableMockStorage{}

	hh := &HeartHandler{}
	nh := &NeuronHandler{}
	mh := &MuscleHandler{}

	reg := NewRegistry()
	reg.Add(hh)
	reg.Add(nh)
	reg.Add(mh)

	err := reg.EngageAll(ctx, store, spine)
	if err != nil {
		t.Fatalf("failed to engage all in concurrent test: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_specialization_concurrent_worker", "runs concurrent event handlers").StartSimple(func() {
			defer wg.Done()
			if err := hh.handleAuditEvent(ctx, infrastructure.Event{Op: "convergence_success"}); err != nil {
				t.Errorf("audit event failed: %v", err)
			}
			if err := hh.handleMetricEvent(ctx, infrastructure.Event{Kind: "metrics"}); err != nil {
				t.Errorf("metric event failed: %v", err)
			}
			if err := nh.handleConvergenceUpdate(ctx, infrastructure.Event{
				Payload: map[string]any{
					objects.FieldKeyStatus:          "active",
					objects.FieldKeyDeltaAssessment: "converged",
				},
			}); err != nil {
				t.Errorf("convergence update failed: %v", err)
			}
			if err := mh.handleBacklogProposal(ctx, infrastructure.Event{
				Op: "propose",
				Payload: map[string]any{
					objects.FieldKeyTitle:        "Concurrent Task",
					objects.FieldKeyPriorityTier: "P2",
				},
			}); err != nil {
				t.Errorf("backlog proposal failed: %v", err)
			}
		})
	}
	wg.Wait()

	reg.DisengageAll()
}
