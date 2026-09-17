package cas_test

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/filecas"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestCASErasePendingTombstoneLinger(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "cas-erase-pending-test-*")
	require.NoError(t, err)
	defer fileutil.RemoveAll(tempDir)

	kindDir := filepath.Join(tempDir, paths.ProcessDir, "criteria")
	require.NoError(t, fileutil.EnsureDir(kindDir))

	cas := filecas.NewContentAddressableStorage(kindDir, "criteria")
	testID := "CRIT-1785784848555610000-7a0f9370"

	// Initially, ID does not exist and is not erase_pending
	assert.False(t, cas.IsErasePending(testID))

	// Set erase_pending with an event ID / reason
	eventID := "EVT-SHOCKWAVE-12345"
	cas.SetErasePending(testID, eventID)
	assert.True(t, cas.IsErasePending(testID))
	gotEvent, ok := cas.GetErasePendingEventID(testID)
	assert.True(t, ok)
	assert.Equal(t, eventID, gotEvent)

	// In index snapshot
	pendingMap := cas.SnapshotErasePending()
	assert.Equal(t, eventID, pendingMap[testID])

	// Clear erase_pending (FINALIZE)
	cas.ClearErasePending(testID)
	assert.False(t, cas.IsErasePending(testID))
	_, okAfter := cas.GetErasePendingEventID(testID)
	assert.False(t, okAfter)
}
