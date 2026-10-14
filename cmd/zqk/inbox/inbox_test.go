package inbox

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clictx "github.com/zqk-os/zqk/pkg/cliapp/context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func executeInbox(t *testing.T, projectRoot string, args ...string) (string, error) {
	t.Helper()
	cmd := NewInboxCmd()
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cliCtx := &cli.Context{
		Context: &clictx.Context{
			ProjectRoot: projectRoot,
			Profile:     "debug",
		},
	}
	cli.SetContext(cmd, cliCtx)
	for _, sub := range cmd.Commands() {
		cli.SetContext(sub, cliCtx)
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func seedInstruction(t *testing.T, store storage.ObjectStorageProvider, id, status, persona, session, instruction, createdAt string) {
	t.Helper()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	obj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          "agent_instruction",
		objects.FieldKeyStatus:        status,
		objects.FieldKeyTargetPersona: persona,
		objects.FieldKeySourceSession: session,
		objects.FieldKeyInstruction:   instruction,
		objects.FieldKeyCreatedAt:     createdAt,
	}
	if err := store.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to seed instruction %s: %v", id, err)
	}
}

func TestNewInboxCmd_Hierarchy(t *testing.T) {
	cmd := NewInboxCmd()
	if cmd == nil {
		t.Fatal("expected NewInboxCmd to return non-nil command")
	}
	if cmd.Use != "inbox" {
		t.Errorf("expected Use='inbox', got %q", cmd.Use)
	}

	var foundList bool
	for _, sub := range cmd.Commands() {
		if sub.Name() == "list" {
			foundList = true
			break
		}
	}
	if !foundList {
		t.Fatal("expected 'list' subcommand under inbox")
	}
}

func TestInbox_List_Empty(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// Default table format on empty inbox
	out, err := executeInbox(t, proj.Root, "list")
	if err != nil {
		t.Fatalf("inbox list failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "Inbox is empty") {
		t.Fatalf("expected 'Inbox is empty' in output, got: %s", out)
	}

	// JSON format on empty inbox
	jsonOut, err := executeInbox(t, proj.Root, "list", "--format", "json")
	if err != nil {
		t.Fatalf("inbox list --format json failed: %v, out: %s", err, jsonOut)
	}
	if strings.TrimSpace(jsonOut) != "[]" && strings.TrimSpace(jsonOut) != "null" {
		t.Fatalf("expected empty json array or null, got: %s", jsonOut)
	}
}

func TestInbox_List_Populated(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	store := proj.FileStorage

	longInstruction := "This is an extremely long instruction intended to verify proper ellipsis truncation beyond forty characters"
	seedInstruction(t, store, "AGI-001", "proposed", "developer", "sess-100", "Fix bug in parser", "2026-10-14T01:00:00Z")
	seedInstruction(t, store, "AGI-002", "approved", "architect", "sess-200", longInstruction, "2026-10-14T02:00:00Z")

	// Table format: verify columns, items, truncation
	out, err := executeInbox(t, proj.Root, "list")
	if err != nil {
		t.Fatalf("inbox list populated failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "AGI-001") || !strings.Contains(out, "AGI-002") {
		t.Fatalf("expected instruction IDs in output, got: %s", out)
	}
	if !strings.Contains(out, "2 item(s)") {
		t.Fatalf("expected '2 item(s)' in output, got: %s", out)
	}
	if !strings.Contains(out, "...") {
		t.Fatalf("expected truncated instruction with '...', got: %s", out)
	}

	// Filter by status
	statusOut, err := executeInbox(t, proj.Root, "list", "--status", "proposed")
	if err != nil {
		t.Fatalf("inbox list --status failed: %v, out: %s", err, statusOut)
	}
	if !strings.Contains(statusOut, "AGI-001") || strings.Contains(statusOut, "AGI-002") {
		t.Fatalf("expected only AGI-001 in status filtered output, got: %s", statusOut)
	}

	// Filter by persona
	personaOut, err := executeInbox(t, proj.Root, "list", "--persona", "architect")
	if err != nil {
		t.Fatalf("inbox list --persona failed: %v, out: %s", err, personaOut)
	}
	if strings.Contains(personaOut, "AGI-001") || !strings.Contains(personaOut, "AGI-002") {
		t.Fatalf("expected only AGI-002 in persona filtered output, got: %s", personaOut)
	}

	// JSON format with items
	jsonOut, err := executeInbox(t, proj.Root, "list", "--format", "json")
	if err != nil {
		t.Fatalf("inbox list --format json failed: %v, out: %s", err, jsonOut)
	}
	if !strings.Contains(jsonOut, "AGI-001") || !strings.Contains(jsonOut, "AGI-002") {
		t.Fatalf("expected items in JSON output, got: %s", jsonOut)
	}
}
