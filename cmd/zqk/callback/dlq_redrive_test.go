package callback

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type mockSubscriber struct {
	mu        sync.Mutex
	name      string
	failUntil int
	callCount int
	failCount int
	received  []*CallbackEntry
}

func newMockSubscriber(name string, failUntil int) *mockSubscriber {
	return &mockSubscriber{
		name:      name,
		failUntil: failUntil,
		received:  make([]*CallbackEntry, 0),
	}
}

func (s *mockSubscriber) Name() string {
	return s.name
}

func (s *mockSubscriber) Notify(ctx context.Context, entry *CallbackEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callCount++
	if s.callCount <= s.failUntil {
		s.failCount++
		return errfmt.Errorf("simulated delivery error %d", s.callCount)
	}
	s.received = append(s.received, entry)
	return nil
}

func (s *mockSubscriber) TotalCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callCount
}

func (s *mockSubscriber) ReceivedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.received)
}

type dlqTestHarness struct {
	manager    *DistributedDLQManager
	subscriber *mockSubscriber
	ctx        context.Context
}

func newDLQHarness(t *testing.T, opts ...DLQOption) *dlqTestHarness {
	t.Helper()
	sub := newMockSubscriber("harness_subscriber", 0)
	mgr := NewDistributedDLQManager(opts...)
	t.Cleanup(func() {
		closeErr := mgr.Close()
		assert.NoError(t, closeErr)
	})
	return &dlqTestHarness{
		manager:    mgr,
		subscriber: sub,
		ctx:        context.Background(),
	}
}

func createTestEntry(t *testing.T, id string, seq int64, topic string) *CallbackEntry {
	t.Helper()
	return &CallbackEntry{
		JobID:     id,
		Seq:       seq,
		Timestamp: time.Now().UTC(),
		Priority:  1,
		Payload: map[string]any{
			"topic":   topic,
			"job_id":  id,
			"payload": fmt.Sprintf("content_for_%s", id),
		},
	}
}

func assertEnvelopeState(t *testing.T, env *QuarantineEnvelope, expectedStatus QuarantineStatus, expectedAttempts int) {
	t.Helper()
	require.NotNil(t, env)
	assert.Equal(t, expectedStatus, env.Status)
	assert.Equal(t, expectedAttempts, env.AttemptCount)
}

func TestDLQ_EnqueueAndChecksumVerification(t *testing.T) {
	harness := newDLQHarness(t)
	entry := createTestEntry(t, "job-101", 1, "topic_alpha")

	env, err := harness.manager.Enqueue(harness.ctx, entry, "sub_1", "initial timeout")
	require.NoError(t, err)
	assertEnvelopeState(t, env, StatusQuarantined, 0)
	assert.Equal(t, env.ID, env.EnvelopeID)
	assert.NotEmpty(t, env.PayloadChecksum)

	integrityErr := ValidateIntegrity(env)
	assert.NoError(t, integrityErr)

	fetched, fetchErr := harness.manager.Get(env.ID)
	require.NoError(t, fetchErr)
	assert.Equal(t, env.ID, fetched.ID)
}

func TestDLQ_SingleEventRedrive_Success(t *testing.T) {
	harness := newDLQHarness(t)
	entry := createTestEntry(t, "job-102", 2, "topic_beta")

	env, err := harness.manager.Enqueue(harness.ctx, entry, "sub_2", "connection refused")
	require.NoError(t, err)

	redriveErr := harness.manager.Redrive(harness.ctx, env.ID, harness.subscriber)
	require.NoError(t, redriveErr)
	assertEnvelopeState(t, env, StatusRedriven, 0)
	assert.Equal(t, 1, harness.subscriber.ReceivedCount())
}

func TestDLQ_PoisonPillIsolation_ExceedsMaxAttempts(t *testing.T) {
	harness := newDLQHarness(t, WithMaxAttempts(3))
	harness.subscriber.failUntil = 10
	entry := createTestEntry(t, "job-103", 3, "topic_poison")

	env, err := harness.manager.Enqueue(harness.ctx, entry, "sub_3", "fatal unmarshal error")
	require.NoError(t, err)

	for attempt := 1; attempt <= 2; attempt++ {
		redriveErr := harness.manager.Redrive(harness.ctx, env.ID, harness.subscriber)
		assert.Error(t, redriveErr)
		assertEnvelopeState(t, env, StatusQuarantined, attempt)
	}

	thirdErr := harness.manager.Redrive(harness.ctx, env.ID, harness.subscriber)
	assert.Error(t, thirdErr)
	assertEnvelopeState(t, env, StatusPoisonPill, 3)

	isolatedPill, inVault := harness.manager.PoisonPills().Get(env.ID)
	assert.True(t, inVault)
	require.NotNil(t, isolatedPill)
	assert.Equal(t, StatusPoisonPill, isolatedPill.Status)

	subsequentErr := harness.manager.Redrive(harness.ctx, env.ID, harness.subscriber)
	assert.ErrorIs(t, subsequentErr, ErrMaxAttemptsExceeded)
}

func TestDLQ_CorruptedEnvelopeRejection(t *testing.T) {
	harness := newDLQHarness(t)
	entry := createTestEntry(t, "job-104", 4, "topic_security")

	env, err := harness.manager.Enqueue(harness.ctx, entry, "sub_4", "network glitch")
	require.NoError(t, err)

	originalChecksum := env.PayloadChecksum
	env.PayloadChecksum = "bad_tampered_checksum_digest_hex_0000"
	corruptedErr := ValidateIntegrity(env)
	assert.ErrorIs(t, corruptedErr, ErrCorruptedEnvelope)

	redriveCorruptedErr := harness.manager.Redrive(harness.ctx, env.ID, harness.subscriber)
	assert.ErrorIs(t, redriveCorruptedErr, ErrCorruptedEnvelope)

	env.PayloadChecksum = originalChecksum
	env.Entry.Payload["unauthorized_field"] = "hacked_value"
	tamperedPayloadErr := ValidateIntegrity(env)
	assert.ErrorIs(t, tamperedPayloadErr, ErrCorruptedEnvelope)
}

func TestDLQ_BatchRedrive(t *testing.T) {
	harness := newDLQHarness(t)
	for i := 1; i <= 5; i++ {
		entry := createTestEntry(t, fmt.Sprintf("job-alpha-%d", i), int64(i), "topic_batch_a")
		enqEnv, err := harness.manager.Enqueue(harness.ctx, entry, "sub_a", "temporary 503")
		require.NoError(t, err)
		require.NotNil(t, enqEnv)
	}
	for i := 1; i <= 3; i++ {
		entry := createTestEntry(t, fmt.Sprintf("job-beta-%d", i), int64(10+i), "topic_batch_b")
		enqEnv, err := harness.manager.Enqueue(harness.ctx, entry, "sub_b", "temporary 503")
		require.NoError(t, err)
		require.NotNil(t, enqEnv)
	}

	filter := DLQFilter{Topic: "topic_batch_a"}
	redriven, batchErr := harness.manager.RedriveBatch(harness.ctx, filter, harness.subscriber)
	require.NoError(t, batchErr)
	assert.Equal(t, 5, len(redriven))

	remainingBeta := harness.manager.List(DLQFilter{Topic: "topic_batch_b"})
	assert.Equal(t, 3, len(remainingBeta))
	for idx := 0; idx < len(remainingBeta); idx++ {
		assert.Equal(t, StatusQuarantined, remainingBeta[idx].Status)
	}
}

func TestDLQ_FIFOEvictionOnCapacityOverflow(t *testing.T) {
	harness := newDLQHarness(t, WithCapacity(3))
	envelopes := make([]*QuarantineEnvelope, 0, 5)

	for i := 1; i <= 5; i++ {
		entry := createTestEntry(t, fmt.Sprintf("job-overflow-%d", i), int64(i), "topic_overflow")
		env, err := harness.manager.Enqueue(harness.ctx, entry, "sub_overflow", "buffer full")
		require.NoError(t, err)
		envelopes = append(envelopes, env)
	}

	stats := harness.manager.Stats()
	assert.Equal(t, 3, stats.ActiveCount)
	assert.Equal(t, int64(2), stats.EvictedTotal)
	assert.Equal(t, StatusExpired, envelopes[0].Status)
	assert.Equal(t, StatusExpired, envelopes[1].Status)
}

func TestDLQ_PurgeAndFilter(t *testing.T) {
	harness := newDLQHarness(t)
	for i := 1; i <= 4; i++ {
		entry := createTestEntry(t, fmt.Sprintf("job-purge-%d", i), int64(i), "topic_purge_target")
		enqEnv, err := harness.manager.Enqueue(harness.ctx, entry, "sub_purge", "drop")
		require.NoError(t, err)
		require.NotNil(t, enqEnv)
	}

	purged, purgeErr := harness.manager.Purge(DLQFilter{Topic: "topic_purge_target"})
	require.NoError(t, purgeErr)
	assert.Equal(t, 4, purged)

	remaining := harness.manager.List(DLQFilter{Topic: "topic_purge_target"})
	assert.Empty(t, remaining)
}

func TestDLQ_AutomatedRedriveEngine(t *testing.T) {
	harness := newDLQHarness(t)
	harness.subscriber.failUntil = 1
	entry := createTestEntry(t, "job-auto-1", 1, "topic_auto")

	env, err := harness.manager.Enqueue(harness.ctx, entry, "sub_auto", "network blip")
	require.NoError(t, err)

	autoErr := harness.manager.StartAutoRedrive(harness.ctx, 20*time.Millisecond, harness.subscriber)
	require.NoError(t, autoErr)

	require.Eventually(t, func() bool {
		fetched, getErr := harness.manager.Get(env.ID)
		if getErr != nil || fetched == nil {
			return false
		}
		return fetched.Status == StatusRedriven
	}, 2*time.Second, 20*time.Millisecond)

	assert.Equal(t, 1, harness.subscriber.ReceivedCount())
}

func TestDLQ_CalculateBackoffAndJitter(t *testing.T) {
	base := 100 * time.Millisecond
	max := 2 * time.Second

	b1 := CalculateBackoff(1, base, max)
	assert.GreaterOrEqual(t, b1, base)
	assert.LessOrEqual(t, b1, max)

	b3 := CalculateBackoff(3, base, max)
	assert.GreaterOrEqual(t, b3, base)

	bHuge := CalculateBackoff(50, base, max)
	assert.LessOrEqual(t, bHuge, max)
}

func TestDLQ_ConcurrentEnqueueAndRedrive_Race(t *testing.T) {
	harness := newDLQHarness(t, WithCapacity(500), WithMaxAttempts(4))
	var wg sync.WaitGroup
	const workerCount = 4
	const itemsPerWorker = 25

	for w := 0; w < workerCount; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine(fmt.Sprintf("dlq_producer_%d", workerID), "concurrent enqueue").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < itemsPerWorker; i++ {
				jobID := fmt.Sprintf("job-worker-%d-%d", workerID, i)
				entry := createTestEntry(t, jobID, int64(i), "topic_race")
				env, enqErr := harness.manager.Enqueue(harness.ctx, entry, "sub_race", "race test")
				assert.NoError(t, enqErr)
				if env != nil {
					redriveErr := harness.manager.Redrive(harness.ctx, env.ID, harness.subscriber)
					if redriveErr != nil {
						assert.Error(t, redriveErr)
					}
				}
			}
		})
	}

	wg.Wait()
	stats := harness.manager.Stats()
	assert.Equal(t, int64(workerCount*itemsPerWorker), stats.EnqueuedTotal)
}
