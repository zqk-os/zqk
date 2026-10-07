package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

type mockHelpApplicator struct {
	applied bool
}

func (m *mockHelpApplicator) ApplyToCommand(cmd *cobra.Command) {
	m.applied = true
}

func TestFinalizeCommand_And_Bare(t *testing.T) {
	// FinalizeCommand
	cmd1 := &cobra.Command{Use: "test1"}
	help1 := &mockHelpApplicator{}
	res1 := FinalizeCommand(cmd1, help1)
	if !help1.applied {
		t.Errorf("expected help applicator to be called")
	}
	if res1.Flags().Lookup(FlagFormat) == nil {
		t.Errorf("expected common flag FlagFormat to be added")
	}

	// FinalizeBareCommand
	cmd2 := &cobra.Command{Use: "test2"}
	help2 := &mockHelpApplicator{}
	res2 := FinalizeBareCommand(cmd2, help2)
	if !help2.applied {
		t.Errorf("expected help applicator to be called")
	}
	if res2.Flags().Lookup(FlagFormat) != nil {
		t.Errorf("expected no common flags on bare command")
	}
}

func TestAddCommonFlagsExcluding(t *testing.T) {
	cmd := &cobra.Command{Use: "exclude-test"}
	AddCommonFlagsExcluding(cmd, []string{FlagTimeout, FlagColumns})

	if cmd.Flags().Lookup(FlagTimeout) != nil {
		t.Errorf("expected FlagTimeout to be excluded")
	}
	if cmd.Flags().Lookup(FlagColumns) != nil {
		t.Errorf("expected FlagColumns to be excluded")
	}
	if cmd.Flags().Lookup(FlagFormat) == nil {
		t.Errorf("expected FlagFormat to be present")
	}
	if cmd.Flags().Lookup(FlagVerbose) == nil {
		t.Errorf("expected FlagVerbose to be present")
	}
}

func TestAddValidationFlags_And_Booleans(t *testing.T) {
	cmd := &cobra.Command{Use: "val-test"}
	AddValidationFlags(cmd)

	if cmd.Flags().Lookup(FlagDryRun) == nil {
		t.Errorf("expected dry-run flag")
	}
	if cmd.Flags().Lookup(FlagForce) == nil {
		t.Errorf("expected force flag")
	}

	if IsDryRun(cmd) {
		t.Errorf("expected IsDryRun=false by default")
	}
	if IsForce(cmd) {
		t.Errorf("expected IsForce=false by default")
	}

	_ = cmd.Flags().Set(FlagDryRun, "true")
	_ = cmd.Flags().Set(FlagForce, "true")

	if !IsDryRun(cmd) {
		t.Errorf("expected IsDryRun=true after set")
	}
	if !IsForce(cmd) {
		t.Errorf("expected IsForce=true after set")
	}
}

func TestFlagInheritanceAndChain(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String(FlagFormat, "json", "")
	root.PersistentFlags().Duration(FlagTimeout, 10*time.Second, "")

	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)

	f := GetFlagFromChain(child, FlagFormat)
	if f == nil || f.Value.String() != "json" {
		t.Errorf("expected inherited format 'json'")
	}

	d := GetTimeout(child)
	if d != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", d)
	}

	if FormatFlagExplicitlySet(child) {
		t.Errorf("expected FormatFlagExplicitlySet=false when flag not explicitly marked changed")
	}
	_ = child.Flags().Set(FlagFormat, "yaml")
	// Marking changed on root flag
	_ = root.PersistentFlags().Set(FlagFormat, "yaml")
	if !FormatFlagExplicitlySet(child) {
		t.Errorf("expected FormatFlagExplicitlySet=true after set")
	}
}

func TestCommandContextOr_And_CommandOutputWriter(t *testing.T) {
	// Nil cmd
	ctx1 := CommandContextOr(nil, nil)
	if ctx1 == nil {
		t.Errorf("expected fallback system context for nil cmd")
	}

	// Cmd with context
	bg := context.Background()
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(bg)
	ctx2 := CommandContextOr(cmd, nil)
	if ctx2 != bg {
		t.Errorf("expected cmd context")
	}

	w := CommandOutputWriter(cmd, nil)
	if w == nil {
		t.Errorf("expected non-nil output writer")
	}
}

func TestFormatOutputAs_And_HandleError(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(ctx)

	payload := map[string]any{"status": "active", "code": 200}
	err := FormatOutputAs(cmd, FormatJSON, payload)
	if err != nil {
		t.Fatalf("FormatOutputAs failed: %v", err)
	}
	if !strings.Contains(buf.String(), "active") {
		t.Errorf("expected JSON output containing 'active', got: %s", buf.String())
	}

	// HandleError
	testErr := errors.New("sample error for handler")
	handledErr := HandleError(cmd, testErr)
	if handledErr == nil || handledErr.Error() != testErr.Error() {
		t.Errorf("expected HandleError to return wrapped or direct error")
	}
}
