package community_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/community"
)

// CRIT-1794503887035469000-088f7c41: Distributed lock manager rejects blank resource key, blank client ID, or zero lease TTL.
func TestEphemeralLockManager_StaticFloorInvariants(t *testing.T) {
	mgr := community.NewEphemeralLockManager()
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

	t.Run("rejects blank resource key", func(t *testing.T) {
		_, err := mgr.Acquire("   ", "client-1", 5*time.Second, now)
		assert.ErrorIs(t, err, community.ErrBlankResourceKey)

		err = mgr.Release(" ", "client-1", now)
		assert.ErrorIs(t, err, community.ErrBlankResourceKey)
	})

	t.Run("rejects blank client ID", func(t *testing.T) {
		_, err := mgr.Acquire("branch/main", "", 5*time.Second, now)
		assert.ErrorIs(t, err, community.ErrBlankLockClientID)

		err = mgr.Release("branch/main", " \t", now)
		assert.ErrorIs(t, err, community.ErrBlankLockClientID)
	})

	t.Run("rejects zero or negative TTL", func(t *testing.T) {
		_, err := mgr.Acquire("branch/main", "client-1", 0, now)
		assert.ErrorIs(t, err, community.ErrInvalidLeaseTTL)

		_, err = mgr.Acquire("branch/main", "client-1", -1*time.Second, now)
		assert.ErrorIs(t, err, community.ErrInvalidLeaseTTL)

		_, err = mgr.ExtendLease("branch/main", "client-1", 0, now)
		assert.ErrorIs(t, err, community.ErrInvalidLeaseTTL)
	})
}

// CRIT-1794503890607472000-8608c4af: Distributed lock manager grants exclusive lease token, extends TTL upon keepalive, and releases cleanly.
func TestEphemeralLockManager_OperationalDynamicProof(t *testing.T) {
	mgr := community.NewEphemeralLockManager()
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

	// 1. Acquire
	rec, err := mgr.Acquire("resource/cas-flush", "agent-alpha", 10*time.Second, now)
	require.NoError(t, err)
	assert.Equal(t, "resource/cas-flush", rec.ResourceKey)
	assert.Equal(t, "agent-alpha", rec.ClientID)
	assert.Equal(t, uint64(1), rec.FencingToken)
	assert.Equal(t, now.Add(10*time.Second), rec.ExpiresAt)

	// 2. Inspect active
	inspected, ok := mgr.Inspect("resource/cas-flush", now.Add(2*time.Second))
	assert.True(t, ok)
	assert.Equal(t, uint64(1), inspected.FencingToken)

	// 3. Extend
	extended, err := mgr.ExtendLease("resource/cas-flush", "agent-alpha", 15*time.Second, now.Add(5*time.Second))
	require.NoError(t, err)
	assert.Equal(t, now.Add(20*time.Second), extended.ExpiresAt)

	// 4. Release
	err = mgr.Release("resource/cas-flush", "agent-alpha", now.Add(6*time.Second))
	require.NoError(t, err)

	_, ok = mgr.Inspect("resource/cas-flush", now.Add(7*time.Second))
	assert.False(t, ok)
}

// CRIT-1794503894480722000-dde54a74: Distributed lock manager prevents lock stealing by competing nodes and auto-reclaims expired dead-agent lease.
func TestEphemeralLockManager_NegativeAdversarialBoundary(t *testing.T) {
	mgr := community.NewEphemeralLockManager()
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

	// Agent 1 acquires 5-second lock
	rec1, err := mgr.Acquire("resource/git-push", "agent-1", 5*time.Second, now)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), rec1.FencingToken)

	// Agent 2 attempts acquisition while unexpired -> rejected
	_, err = mgr.Acquire("resource/git-push", "agent-2", 5*time.Second, now.Add(2*time.Second))
	assert.ErrorIs(t, err, community.ErrLockHeldByAnother)

	// Agent 2 attempts release of Agent 1's lock -> rejected
	err = mgr.Release("resource/git-push", "agent-2", now.Add(2*time.Second))
	assert.ErrorIs(t, err, community.ErrLockNotOwned)

	// Agent 2 attempts extend of Agent 1's lock -> rejected
	_, err = mgr.ExtendLease("resource/git-push", "agent-2", 10*time.Second, now.Add(2*time.Second))
	assert.ErrorIs(t, err, community.ErrLockNotOwned)

	// Time advances past expiry (dead-agent timeout)
	afterExpiry := now.Add(6 * time.Second)

	// Inspect confirms expired
	_, ok := mgr.Inspect("resource/git-push", afterExpiry)
	assert.False(t, ok)

	// Agent 2 acquires reclaimed resource -> succeeds with next monotonic fencing token
	rec2, err := mgr.Acquire("resource/git-push", "agent-2", 5*time.Second, afterExpiry)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), rec2.FencingToken)
	assert.Equal(t, "agent-2", rec2.ClientID)
}
