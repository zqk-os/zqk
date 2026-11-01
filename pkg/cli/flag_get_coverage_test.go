package cli

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestFlagBag_ExtendedGetters(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("str-whitespace", "   trimmed value   ", "")
	cmd.Flags().String("str-empty", "     ", "")
	cmd.Flags().Int("int-positive", 42, "")
	cmd.Flags().Int("int-zero", 0, "")
	cmd.Flags().Int("int-neg", -5, "")
	cmd.Flags().StringSlice("slice-mixed", []string{"  alpha ", "", "   ", "beta  "}, "")
	cmd.Flags().StringSlice("slice-empty", []string{}, "")
	cmd.Flags().Duration("dur-positive", 5*time.Second, "")
	cmd.Flags().Duration("dur-zero", 0, "")
	cmd.Flags().Duration("dur-neg", -10*time.Second, "")
	cmd.Flags().String("file", "/path/to/spec.yaml", "")
	cmd.Flags().String("data", "{\"key\":\"val\"}", "")
	cmd.Flags().StringArray("field", []string{"f1", "f2"}, "")

	var f FlagBag

	// StringTrimmed
	if got := f.StringTrimmed(cmd, "str-whitespace"); got != "trimmed value" {
		t.Fatalf("StringTrimmed got %q, want %q", got, "trimmed value")
	}

	// StringTrimmedOrDefault
	if got := f.StringTrimmedOrDefault(cmd, "str-empty", "fallback"); got != "fallback" {
		t.Fatalf("StringTrimmedOrDefault got %q, want %q", got, "fallback")
	}
	if got := f.StringTrimmedOrDefault(cmd, "str-whitespace", "fallback"); got != "trimmed value" {
		t.Fatalf("StringTrimmedOrDefault got %q, want %q", got, "trimmed value")
	}

	// IntOrDefault
	if got := f.IntOrDefault(cmd, "int-positive", 99); got != 42 {
		t.Fatalf("IntOrDefault positive got %d, want 42", got)
	}
	if got := f.IntOrDefault(cmd, "int-zero", 99); got != 99 {
		t.Fatalf("IntOrDefault zero got %d, want 99", got)
	}
	if got := f.IntOrDefault(cmd, "int-neg", 99); got != 99 {
		t.Fatalf("IntOrDefault neg got %d, want 99", got)
	}

	// StringSliceTrimmed
	trimmedSlice := f.StringSliceTrimmed(cmd, "slice-mixed")
	if len(trimmedSlice) != 2 || trimmedSlice[0] != "alpha" || trimmedSlice[1] != "beta" {
		t.Fatalf("StringSliceTrimmed unexpected result: %v", trimmedSlice)
	}
	emptySlice := f.StringSliceTrimmed(cmd, "slice-empty")
	if emptySlice != nil {
		t.Fatalf("StringSliceTrimmed empty expected nil, got %v", emptySlice)
	}

	// DurationOrDefault
	if got := f.DurationOrDefault(cmd, "dur-positive", 10*time.Second); got != 5*time.Second {
		t.Fatalf("DurationOrDefault positive got %v, want 5s", got)
	}
	if got := f.DurationOrDefault(cmd, "dur-zero", 10*time.Second); got != 10*time.Second {
		t.Fatalf("DurationOrDefault zero got %v, want 10s", got)
	}
	if got := f.DurationOrDefault(cmd, "dur-neg", 10*time.Second); got != 10*time.Second {
		t.Fatalf("DurationOrDefault neg got %v, want 10s", got)
	}

	// ReadDataInputFlags
	fp, data, fields := f.ReadDataInputFlags(cmd)
	if fp != "/path/to/spec.yaml" || data != "{\"key\":\"val\"}" || len(fields) != 2 {
		t.Fatalf("ReadDataInputFlags failed: fp=%q, data=%q, fields=%v", fp, data, fields)
	}

	if f.Err() != nil {
		t.Fatalf("unexpected FlagBag error: %v", f.Err())
	}
}

func TestFlagBag_NilCommandSafety(t *testing.T) {
	var f FlagBag
	if got := f.StringSliceTrimmed(nil, "flag"); got != nil {
		t.Fatalf("expected nil from StringSliceTrimmed on nil cmd, got %v", got)
	}
	if got := f.DurationOrDefault(nil, "flag", 5*time.Second); got != 5*time.Second {
		t.Fatalf("expected default duration on nil cmd, got %v", got)
	}
	if got := f.IntOrDefault(nil, "flag", 10); got != 10 {
		t.Fatalf("expected default int on nil cmd, got %v", got)
	}
	fp, data, fields := f.ReadDataInputFlags(nil)
	if fp != "" || data != "" || fields != nil {
		t.Fatalf("expected empty values on nil cmd, got fp=%q, data=%q, fields=%v", fp, data, fields)
	}
}

func TestTimeoutHook_HexAndNormalization(t *testing.T) {
	// isHexString proofs
	if !isHexString("0123456789abcdefABCDEF") {
		t.Fatal("isHexString failed on valid hex")
	}
	if isHexString("nothex0123") {
		t.Fatal("isHexString accepted invalid characters")
	}
	if isHexString("1234z") {
		t.Fatal("isHexString accepted 'z'")
	}

	hook := NewTimeoutHook()

	// isVariableArg proofs
	sha1Hash := "da39a3ee5e6b4b0d3255bfef95601890afd80709"
	sha256Hash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if !hook.isVariableArg(sha1Hash) {
		t.Fatal("isVariableArg failed to recognize SHA1 hash")
	}
	if !hook.isVariableArg(sha256Hash) {
		t.Fatal("isVariableArg failed to recognize SHA256 hash")
	}
	if !hook.isVariableArg("--title=some-title") {
		t.Fatal("isVariableArg failed to recognize flag assignment")
	}
	if !hook.isVariableArg("123456") {
		t.Fatal("isVariableArg failed to recognize numeric literal")
	}
	if !hook.isVariableArg("BLI-1793575952828368500-e75907dd") {
		t.Fatal("isVariableArg failed to recognize kernel nanosecond ID")
	}
	if !hook.isVariableArg("/tmp/test.txt") {
		t.Fatal("isVariableArg failed to recognize file path")
	}
	if hook.isVariableArg("status") {
		t.Fatal("isVariableArg falsely recognized static command 'status'")
	}
	if hook.isVariableArg("") {
		t.Fatal("isVariableArg falsely recognized empty argument")
	}

	// normalizeCommand proofs
	cmdLine := hook.normalizeCommand("zqk", []string{"object", "get", "BLI-123", "--format=json", "status"})
	expected := "zqk object get <arg> <arg> status"
	if cmdLine != expected {
		t.Fatalf("normalizeCommand got %q, want %q", cmdLine, expected)
	}

	// sanitizeArgs proofs
	sanitized := hook.sanitizeArgs([]string{"system", "--token", "secret", "status"})
	if len(sanitized) != 4 || sanitized[1] != "--token" {
		t.Fatalf("sanitizeArgs returned unexpected: %v", sanitized)
	}
}

func TestTimeoutHook_ExemptionsAndCalculations(t *testing.T) {
	hook := NewTimeoutHook()

	// isChildMaxTimeoutExempt
	if hook.isChildMaxTimeoutExempt("unknown-command", false) {
		t.Fatal("expected unknown-command not to be exempt")
	}

	idle := hook.getIdleShutdownDuration("unknown-command", 5*time.Second)
	if idle != 5*time.Second {
		t.Fatalf("expected default idle shutdown duration 5s, got %v", idle)
	}

	est := hook.estimateObjectCountForSystemCheck([]string{"BLI-1234", "REQ-5678", "--flag"})
	if est != 2 {
		t.Fatalf("expected object count 2, got %d", est)
	}

	estDefault := hook.estimateObjectCountForSystemCheck([]string{"--full"})
	if estDefault <= 0 {
		t.Fatalf("expected positive default estimate, got %d", estDefault)
	}
}

func TestColors_StandardUIPalette(t *testing.T) {
	palette := StandardUIPalette()
	if palette.Cyan == nil || palette.Green == nil || palette.Yellow == nil || palette.Bold == nil {
		t.Fatal("expected all palette formatters to be initialized")
	}
	cyanText := palette.Cyan("test")
	if cyanText == "" {
		t.Fatal("expected non-empty formatted cyan string")
	}
}

func TestAutoExec_PriorityRank(t *testing.T) {
	ranks := map[string]int{
		"p0":       4,
		"urgent":   4,
		"critical": 4,
		"P1":       3,
		"high":     3,
		"p2":       2,
		"normal":   2,
		"medium":   2,
		"p3":       1,
		"low":      1,
		"p4":       0,
		"unknown":  0,
		"":         0,
	}

	for input, expected := range ranks {
		got := priorityRank(input)
		if got != expected {
			t.Fatalf("priorityRank(%q) = %d, want %d", input, got, expected)
		}
	}
	if got := priorityRank(123); got != 0 {
		t.Fatalf("priorityRank(non-string) = %d, want 0", got)
	}
}

func TestCommandBuilder_WithAliases(t *testing.T) {
	b := NewCommandBuilder("custom").
		WithUse("custom").
		WithAliases("c", "cust")

	if len(b.aliases) != 2 || b.aliases[0] != "c" || b.aliases[1] != "cust" {
		t.Fatalf("WithAliases failed to record aliases: %v", b.aliases)
	}
}

func TestCommandSpec_EquivalentCommandIDs(t *testing.T) {
	cases := []struct {
		id       string
		expected string
	}{
		{"do", "workflow_do"},
		{"workflow_do", "do"},
		{"inspect", "object_inspect"},
		{"object_inspect", "inspect"},
		{"mutate", "object_mutate"},
		{"object_mutate", "mutate"},
		{"query", "graph_query"},
		{"graph_query", "query"},
		{"rollback", "object_rollback"},
		{"object_rollback", "rollback"},
	}

	for _, tc := range cases {
		equiv := EquivalentCommandIDs(tc.id)
		found := false
		for _, cand := range equiv {
			if cand == tc.expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("EquivalentCommandIDs(%q) did not contain %q: %v", tc.id, tc.expected, equiv)
		}
	}
}

