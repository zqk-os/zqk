package inbox

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	clictx "github.com/zqk-os/zqk/pkg/cliapp/context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// cycle102MockStorageProvider allows intercepting List calls for error simulation.
type cycle102MockStorageProvider struct {
	storage.ObjectStorageProvider
	listFunc func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error)
}

func (m *cycle102MockStorageProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, secCtx, storageCtx, filter)
	}
	return m.ObjectStorageProvider.List(ctx, secCtx, storageCtx, filter)
}

// TestCycle102_InboxCmd_Structure verifies hierarchy, metadata, help output, and subcommand wiring.
func TestCycle102_InboxCmd_Structure(t *testing.T) {
	cmd := NewInboxCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "inbox", cmd.Use)
	assert.NotEmpty(t, cmd.Short)

	listCmd := NewListCmd()
	require.NotNil(t, listCmd)
	assert.Equal(t, "list", listCmd.Name())
	assert.NotEmpty(t, listCmd.Short)
	assert.NotNil(t, listCmd.Flags().Lookup("status"))
	assert.NotNil(t, listCmd.Flags().Lookup("persona"))

	// Test help execution
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "inbox")
	assert.Contains(t, buf.String(), "list")
}

// TestCycle102_InboxList_NilContext verifies error returned when CLI context is not initialized.
func TestCycle102_InboxList_NilContext(t *testing.T) {
	_ = testkit.PrepareIsolatedTempProject(t, nil)

	cmd := NewListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Explicitly register nil context for this command so GetContext returns nil
	cli.SetContext(cmd, nil)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get context")
}

// TestCycle102_InboxList_StorageError verifies error returned when project root is invalid or missing.
func TestCycle102_InboxList_StorageError(t *testing.T) {
	_ = testkit.PrepareIsolatedTempProject(t, nil)

	cmd := NewListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cliCtx := &cli.Context{
		Context: &clictx.Context{
			ProjectRoot: "", // empty root triggers ErrStorageNotInContext
			Profile:     "debug",
		},
	}
	cli.SetContext(cmd, cliCtx)
	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get storage")
}

// TestCycle102_InboxList_StorageListFailure verifies error handling when storage.List returns an error.
func TestCycle102_InboxList_StorageListFailure(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	mockStore := &cycle102MockStorageProvider{
		ObjectStorageProvider: proj.FileStorage,
		listFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			return nil, errors.New("simulated storage failure")
		},
	}

	cmd := NewListCmd()
	var buf bytes.Buffer
	baseCtx := cli.WithStorageProvider(context.Background(), mockStore)
	cmdCtx := pkgctx.WithCommandOutputWriter(baseCtx, &buf)
	cmd.SetContext(cmdCtx)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cliCtx := &cli.Context{
		Context: &clictx.Context{
			ProjectRoot: proj.Root,
			Profile:     "debug",
		},
	}
	cli.SetContext(cmd, cliCtx)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query inbox")
	assert.Contains(t, err.Error(), "simulated storage failure")
}

// TestCycle102_InboxList_JSONMarshalError verifies error handling when json marshaling fails.
func TestCycle102_InboxList_JSONMarshalError(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	mockStore := &cycle102MockStorageProvider{
		ObjectStorageProvider: proj.FileStorage,
		listFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{
				Objects: []map[string]any{
					{
						objects.FieldKeyID: "AGI-ERR-001",
						"unmarshalable":    make(chan int),
					},
				},
			}, nil
		},
	}

	cmd := NewListCmd()
	var buf bytes.Buffer
	baseCtx := cli.WithStorageProvider(context.Background(), mockStore)
	cmdCtx := pkgctx.WithCommandOutputWriter(baseCtx, &buf)
	cmd.SetContext(cmdCtx)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cliCtx := &cli.Context{
		Context: &clictx.Context{
			ProjectRoot: proj.Root,
			Profile:     "debug",
		},
	}
	cli.SetContext(cmd, cliCtx)
	cmd.SetArgs([]string{"--format", "json"})
	require.NoError(t, cmd.Flags().Set("format", "json"))

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal results")
}

// TestCycle102_InboxList_FilterPermutations verifies all combinations of status and persona filters.
func TestCycle102_InboxList_FilterPermutations(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	store := proj.FileStorage

	seedInstruction(t, store, "AGI-DEMO-001", "proposed", "PER-COMMUNITY-CODE-CRAFTSMAN", "sess-1", "Task 1", "2026-10-20T10:00:00Z")
	seedInstruction(t, store, "AGI-DEMO-002", "proposed", "PER-COMMUNITY-ARCH-DESIGN", "sess-2", "Task 2", "2026-10-20T10:01:00Z")
	seedInstruction(t, store, "AGI-DEMO-003", "approved", "PER-COMMUNITY-CODE-CRAFTSMAN", "sess-3", "Task 3", "2026-10-20T10:02:00Z")
	seedInstruction(t, store, "AGI-DEMO-004", "rejected", "PER-COMMUNITY-QA-VERIFICATION", "sess-4", "Task 4", "2026-10-20T10:03:00Z")

	// 1. Filter by both status AND persona (exact match)
	out, err := executeInbox(t, proj.Root, "list", "--status", "proposed", "--persona", "PER-COMMUNITY-CODE-CRAFTSMAN")
	require.NoError(t, err)
	assert.Contains(t, out, "AGI-DEMO-001")
	assert.NotContains(t, out, "AGI-DEMO-002")
	assert.NotContains(t, out, "AGI-DEMO-003")
	assert.NotContains(t, out, "AGI-DEMO-004")
	assert.Contains(t, out, "1 item(s)")

	// 2. Filter by status only (multiple matches)
	outStatus, err := executeInbox(t, proj.Root, "list", "--status", "proposed")
	require.NoError(t, err)
	assert.Contains(t, outStatus, "AGI-DEMO-001")
	assert.Contains(t, outStatus, "AGI-DEMO-002")
	assert.NotContains(t, outStatus, "AGI-DEMO-003")
	assert.NotContains(t, outStatus, "AGI-DEMO-004")
	assert.Contains(t, outStatus, "2 item(s)")

	// 3. Filter by persona only (multiple matches)
	outPersona, err := executeInbox(t, proj.Root, "list", "--persona", "PER-COMMUNITY-CODE-CRAFTSMAN")
	require.NoError(t, err)
	assert.Contains(t, outPersona, "AGI-DEMO-001")
	assert.NotContains(t, outPersona, "AGI-DEMO-002")
	assert.Contains(t, outPersona, "AGI-DEMO-003")
	assert.NotContains(t, outPersona, "AGI-DEMO-004")
	assert.Contains(t, outPersona, "2 item(s)")

	// 4. Non-matching filter combination yields empty inbox
	outNonMatch, err := executeInbox(t, proj.Root, "list", "--status", "proposed", "--persona", "PER-COMMUNITY-QA-VERIFICATION")
	require.NoError(t, err)
	assert.Contains(t, outNonMatch, "Inbox is empty. No agent instructions pending.")

	// 5. Short flag aliases: -s and -p
	outShortFlags, err := executeInbox(t, proj.Root, "list", "-s", "approved", "-p", "PER-COMMUNITY-CODE-CRAFTSMAN")
	require.NoError(t, err)
	assert.Contains(t, outShortFlags, "AGI-DEMO-003")
	assert.Contains(t, outShortFlags, "1 item(s)")
}

// TestCycle102_InboxList_SortingInvariants verifies reverse chronological sorting (newest first).
func TestCycle102_InboxList_SortingInvariants(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	store := proj.FileStorage

	// Insert in non-chronological order
	seedInstruction(t, store, "AGI-EARLY", "proposed", "dev", "s1", "First instruction", "2026-10-20T08:00:00Z")
	seedInstruction(t, store, "AGI-LATEST", "proposed", "dev", "s3", "Third instruction", "2026-10-20T12:00:00Z")
	seedInstruction(t, store, "AGI-MIDDLE", "proposed", "dev", "s2", "Second instruction", "2026-10-20T10:00:00Z")

	out, err := executeInbox(t, proj.Root, "list")
	require.NoError(t, err)

	idxLatest := strings.Index(out, "AGI-LATEST")
	idxMiddle := strings.Index(out, "AGI-MIDDLE")
	idxEarly := strings.Index(out, "AGI-EARLY")

	require.True(t, idxLatest >= 0, "AGI-LATEST must be present")
	require.True(t, idxMiddle >= 0, "AGI-MIDDLE must be present")
	require.True(t, idxEarly >= 0, "AGI-EARLY must be present")

	// Must be ordered: LATEST < MIDDLE < EARLY in output
	assert.True(t, idxLatest < idxMiddle, "LATEST must appear before MIDDLE")
	assert.True(t, idxMiddle < idxEarly, "MIDDLE must appear before EARLY")
}

// TestCycle102_InboxList_FormattingAndTruncation verifies boundary cases of instruction length and formatting.
func TestCycle102_InboxList_FormattingAndTruncation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	store := proj.FileStorage

	// Case 1: Short instruction (<= 40 chars)
	shortInstr := "Short instruction"
	seedInstruction(t, store, "AGI-SHORT", "proposed", "dev", "s1", shortInstr, "2026-10-20T01:00:00Z")

	// Case 2: Exact boundary (exactly 40 chars)
	exact40Instr := "1234567890123456789012345678901234567890"
	require.Equal(t, 40, len(exact40Instr))
	seedInstruction(t, store, "AGI-EXACT40", "proposed", "dev", "s2", exact40Instr, "2026-10-20T02:00:00Z")

	// Case 3: Over boundary (41 chars)
	over40Instr := "1234567890123456789012345678901234567890X"
	require.Equal(t, 41, len(over40Instr))
	seedInstruction(t, store, "AGI-OVER40", "proposed", "dev", "s3", over40Instr, "2026-10-20T03:00:00Z")

	out, err := executeInbox(t, proj.Root, "list")
	require.NoError(t, err)

	// Short instruction should appear untruncated without ellipsis
	assert.Contains(t, out, shortInstr)

	// Exactly 40 characters should appear untruncated without ellipsis
	assert.Contains(t, out, exact40Instr)

	// 41 characters should be truncated to first 37 chars + "..."
	expectedTruncated := over40Instr[:37] + "..."
	assert.Contains(t, out, expectedTruncated)
	assert.NotContains(t, out, over40Instr)

	// Verify table header format
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "STATUS")
	assert.Contains(t, out, "PERSONA")
	assert.Contains(t, out, "SESSION")
	assert.Contains(t, out, "INSTRUCTION")
	assert.Contains(t, out, strings.Repeat("─", 120))
	assert.Contains(t, out, "3 item(s)")
}

// TestCycle102_InboxList_EdgeCaseFields verifies robust handling of missing or nil fields.
func TestCycle102_InboxList_EdgeCaseFields(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	store := proj.FileStorage

	// Insert object with minimal valid fields (empty target_persona and source_session)
	seedInstruction(t, store, "AGI-SPARSE-001", "proposed", "", "", "Sparse test", "2026-10-20T01:00:00Z")

	out, err := executeInbox(t, proj.Root, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "AGI-SPARSE-001")
	assert.Contains(t, out, "1 item(s)")

	// JSON format should also succeed
	jsonOut, err := executeInbox(t, proj.Root, "list", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, jsonOut, "AGI-SPARSE-001")
}
