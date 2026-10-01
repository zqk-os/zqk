package systemcheck_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/systemcheck"
)

func TestHashMismatchInfo_JSON(t *testing.T) {
	info := systemcheck.HashMismatchInfo{
		ObjectID:     "OBJ-123",
		Kind:         "backlog_item",
		FilePath:     "test/path.yaml",
		OriginalHash: "abc123hash",
	}

	data, err := json.Marshal(info)
	require.NoError(t, err)

	var decoded systemcheck.HashMismatchInfo
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, info, decoded)
}

func TestCheckResultDiff_JSON(t *testing.T) {
	diff := systemcheck.CheckResultDiff{
		ObjectID: "OBJ-456",
		Baseline: systemcheck.CheckResult{
			ObjectID:   "OBJ-456",
			ObjectKind: "goal",
			FilePath:   "goals/g1.yaml",
		},
		Expanded: systemcheck.CheckResult{
			ObjectID:   "OBJ-456",
			ObjectKind: "goal",
			FilePath:   "goals/g1.yaml",
			Status:     "complete",
		},
	}

	data, err := json.Marshal(diff)
	require.NoError(t, err)

	var decoded systemcheck.CheckResultDiff
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, diff, decoded)
}
