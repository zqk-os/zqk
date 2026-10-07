package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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

func TestHelpers_ProfileValidationAndStream(t *testing.T) {
	// ValidateFormat
	if err := ValidateFormat(FormatJSON); err != nil {
		t.Errorf("expected valid JSON format: %v", err)
	}
	if err := ValidateFormat(OutputFormat("unknown_xyz")); err == nil {
		t.Errorf("expected error for invalid format")
	}

	// GetProfile & IsQuiet
	cmd := &cobra.Command{Use: "test"}
	cliCtx := ContextForProjectRoot(".")
	cliCtx.Context.Profile = "ai-agent"
	cliCtx.Context.Quiet = true
	SetContext(cmd, cliCtx)
	if prof := GetProfile(cmd); prof != "ai-agent" {
		t.Errorf("expected 'ai-agent', got %q", prof)
	}
	if !IsQuiet(cmd) {
		t.Errorf("expected IsQuiet=true")
	}
	if IsVerbose(cmd) {
		t.Errorf("expected IsVerbose=false")
	}
	cmd.Flags().String(FlagTimeout, "5s", "")
	if d := GetTimeout(cmd); d <= 0 {
		t.Errorf("expected positive timeout, got %v", d)
	}

	// HandleError and EnhanceError
	if err := HandleError(cmd, nil); err != nil {
		t.Errorf("expected nil from HandleError(nil)")
	}
	sampleErr := errors.New("sample flag error: unknown flag --field")
	if err := HandleError(cmd, sampleErr); err == nil {
		t.Errorf("expected enhanced error from HandleError")
	}

	// CreateContextWithLoggingProfile
	ctx := CreateContextWithLoggingProfile(nil, "mcp")
	if ctx == nil {
		t.Errorf("expected non-nil context for mcp logging profile")
	}
	ctxSys := CreateContextWithLoggingProfile(context.Background(), "system")
	if ctxSys == nil {
		t.Errorf("expected non-nil context for system logging profile")
	}
	ctxDebug := CreateContextWithLoggingProfile(context.Background(), "debug")
	if ctxDebug == nil {
		t.Errorf("expected non-nil context for debug logging profile")
	}
	ctxHuman := CreateContextWithLoggingProfile(context.Background(), "human")
	if ctxHuman == nil {
		t.Errorf("expected non-nil context for human logging profile")
	}

	// ApplyFlagErrorSuggestions
	rootCmd := &cobra.Command{Use: "root"}
	subCmd := &cobra.Command{Use: "sub"}
	rootCmd.AddCommand(subCmd)
	ApplyFlagErrorSuggestions(rootCmd)
	ApplyFlagErrorSuggestions(nil) // guard

	// WriteOutputStream to stdout (empty output path)
	written := false
	err := WriteOutputStream(cmd, func(w io.Writer) error {
		written = true
		return nil
	})
	if err != nil || !written {
		t.Errorf("expected successful WriteOutputStream to default writer")
	}
}

func TestReadRequiredFileFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("file", "", "")

	// Missing/empty flag
	_, err := ReadRequiredFileFlag(cmd, "file")
	if err == nil {
		t.Errorf("expected error for empty --file flag")
	}

	// Valid temporary file
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	if err := fileutil.WriteFile(tmpFile, []byte("file content here"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Flags().Set("file", tmpFile)
	data, err := ReadRequiredFileFlag(cmd, "file")
	if err != nil || string(data) != "file content here" {
		t.Errorf("expected 'file content here', got %q (err=%v)", string(data), err)
	}
}

type mockChecker struct {
	allowPerm    bool
	permReason   string
	allowFormat  bool
	formatReason string
	allowData    bool
	dataReason   string
}

func (m *mockChecker) CheckPermission(ctx context.Context, op, res string) (bool, string) {
	return m.allowPerm, m.permReason
}

func (m *mockChecker) CheckFormatPermission(ctx context.Context, f OutputFormat) (bool, string) {
	return m.allowFormat, m.formatReason
}

func (m *mockChecker) CheckDataAccess(ctx context.Context, d any) (bool, string) {
	return m.allowData, m.dataReason
}

func TestFormatOutputWithPermissionChecker(t *testing.T) {
	cmd := &cobra.Command{Use: "perm-test"}
	cmd.Flags().String("format", "json", "")

	// 1. Success without checker
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	if err := FormatOutputWithPermissionChecker(cmd, map[string]any{"ok": true}, nil); err != nil {
		t.Fatalf("unexpected error without checker: %v", err)
	}

	// 2. Data access denied on streaming format
	_ = cmd.Flags().Set("format", "json-rpc")
	deniedDataChecker := &mockChecker{allowFormat: true, allowData: false, dataReason: "restricted"}
	if err := FormatOutputWithPermissionChecker(cmd, map[string]any{"ok": true}, deniedDataChecker); err == nil {
		t.Errorf("expected error when data access is denied")
	}

	// 3. Format permission denied on streaming format
	deniedFormatChecker := &mockChecker{allowData: true, allowFormat: false, formatReason: "format forbidden"}
	if err := FormatOutputWithPermissionChecker(cmd, map[string]any{"ok": true}, deniedFormatChecker); err == nil {
		t.Errorf("expected error when format permission is denied")
	}
}

func TestWriteOutputStream_ToFile(t *testing.T) {
	cmd := &cobra.Command{Use: "file-out"}
	target := filepath.Join(t.TempDir(), "stream-out.txt")
	cmd.Flags().String("output", target, "")

	err := WriteOutputStream(cmd, func(w io.Writer) error {
		_, writeErr := w.Write([]byte("streaming content"))
		return writeErr
	})
	if err != nil {
		t.Fatalf("WriteOutputStream failed: %v", err)
	}

	data, err := fileutil.ReadFile(target)
	if err != nil || string(data) != "streaming content" {
		t.Errorf("expected 'streaming content', got %q", string(data))
	}
}

func TestWriteOutput_FileAndDash(t *testing.T) {
	// "-" path
	cmdDash := &cobra.Command{Use: "dash"}
	cmdDash.Flags().String("output", "-", "")
	var buf bytes.Buffer
	cmdDash.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	if err := WriteOutput(cmdDash, []byte("dash-data\n")); err != nil {
		t.Fatalf("WriteOutput to dash failed: %v", err)
	}
	if buf.String() != "dash-data\n" {
		t.Errorf("expected 'dash-data\\n', got %q", buf.String())
	}

	// file path
	cmdFile := &cobra.Command{Use: "file"}
	outPath := filepath.Join(t.TempDir(), "file.txt")
	cmdFile.Flags().String("output", outPath, "")
	if err := WriteOutput(cmdFile, []byte("file-data")); err != nil {
		t.Fatalf("WriteOutput to file failed: %v", err)
	}
	read, err := fileutil.ReadFile(outPath)
	if err != nil || string(read) != "file-data" {
		t.Errorf("expected 'file-data', got %q", string(read))
	}
}
