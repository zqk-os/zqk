package workflow

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestNewLinkCmd(t *testing.T) {
	cmd := NewLinkCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "link", cmd.Use)
}

func TestExtractTargetCriteria_Extended(t *testing.T) {
	// 1. Goal with criteria
	goalObj := map[string]any{
		objects.FieldKeyKind:         objects.KindGoal,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-GOAL"},
	}
	id, err := extractTargetCriteria("GOAL-1", goalObj)
	require.NoError(t, err)
	assert.Equal(t, "CRIT-GOAL", id)

	// 2. Goal without criteria
	goalEmpty := map[string]any{
		objects.FieldKeyKind: objects.KindGoal,
	}
	_, err = extractTargetCriteria("GOAL-EMPTY", goalEmpty)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has no criteria established yet")

	// 3. Milestone with criteria
	msObj := map[string]any{
		objects.FieldKeyKind:         objects.KindMilestone,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-MS"},
	}
	id, err = extractTargetCriteria("MIL-1", msObj)
	require.NoError(t, err)
	assert.Equal(t, "CRIT-MS", id)

	// 4. Unsupported kind
	bliObj := map[string]any{
		objects.FieldKeyKind: objects.KindBacklogItem,
	}
	_, err = extractTargetCriteria("BLI-1", bliObj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported pipeline reference kind")
}

type mockLinkStorage struct {
	storage.ObjectStorageProvider
	data map[string]map[string]any
}

func (m *mockLinkStorage) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.data[id]; ok {
		return obj, nil
	}
	return nil, fmt.Errorf("object %s not found", id)
}

func (m *mockLinkStorage) Update(ctx context.Context, sec *pkgctx.SecurityContext, id string, obj map[string]any) error {
	m.data[id] = obj
	return nil
}

func TestRunLinkCommand(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockLinkStorage{
		data: map[string]map[string]any{
			"CRIT-LINK-001": {
				objects.FieldKeyID:          "CRIT-LINK-001",
				objects.FieldKeyKind:        objects.KindCriteria,
				objects.FieldKeyTitle:       "Link Criteria",
				objects.FieldKeyStatus:      objects.ObjectStatusValidated,
				objects.FieldKeyNamespaceID: "zqk:kernel",
			},
			"BLI-LINK-001": {
				objects.FieldKeyID:          "BLI-LINK-001",
				objects.FieldKeyKind:        objects.KindBacklogItem,
				objects.FieldKeyTitle:       "Link Backlog Item",
				objects.FieldKeyStatus:      objects.ObjectStatusPlanned,
				objects.FieldKeyNamespaceID: "zqk:kernel",
			},
			"BLI-LINK-002": {
				objects.FieldKeyID:           "BLI-LINK-002",
				objects.FieldKeyKind:         objects.KindBacklogItem,
				objects.FieldKeyTitle:        "Already Linked BLI",
				objects.FieldKeyStatus:       objects.ObjectStatusPlanned,
				objects.FieldKeyCriteriaRefs: []any{"CRIT-LINK-001"},
				objects.FieldKeyNamespaceID:  "zqk:kernel",
			},
			"GOAL-NO-CRIT": {
				objects.FieldKeyID:          "GOAL-NO-CRIT",
				objects.FieldKeyKind:        objects.KindGoal,
				objects.FieldKeyTitle:       "Goal without crit",
				objects.FieldKeyStatus:      objects.ObjectStatusActive,
				objects.FieldKeyNamespaceID: "zqk:kernel",
			},
		},
	}

	cmd := NewLinkCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	initTestCmdContext(t, cmd, proj.Root, store)

	// 1. Missing arguments
	err := runLinkCommand(cmd, []string{"BLI-LINK-001"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing arguments")

	// 2. Target pipeline ref not found
	err = runLinkCommand(cmd, []string{"BLI-LINK-001", "NON-EXISTENT-CRIT"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load pipeline reference")

	// 3. Target pipeline has no criteria
	err = runLinkCommand(cmd, []string{"BLI-LINK-001", "GOAL-NO-CRIT"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has no criteria established yet")

	// 4. Object to link not found
	err = runLinkCommand(cmd, []string{"NON-EXISTENT-BLI", "CRIT-LINK-001"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load object")

	// 5. Already linked object
	outBuf.Reset()
	err = runLinkCommand(cmd, []string{"BLI-LINK-002", "CRIT-LINK-001"})
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "already linked to criteria")

	// 6. Successful link
	outBuf.Reset()
	err = runLinkCommand(cmd, []string{"BLI-LINK-001", "CRIT-LINK-001"})
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "Successfully linked BLI-LINK-001 into pipeline")

	// Verify criteria_refs was updated on BLI-LINK-001
	readBLI, err := store.Read(context.Background(), nil, "BLI-LINK-001")
	require.NoError(t, err)
	refs, ok := readBLI[objects.FieldKeyCriteriaRefs].([]any)
	require.True(t, ok)
	assert.Contains(t, refs, "CRIT-LINK-001")
}
