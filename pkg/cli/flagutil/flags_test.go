package flagutil

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestFlagUtil_StringTrimmed(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.Flags().String("agent-id", "  peer-agent-1  ", "")
	cmd.Flags().String("empty-flag", "   ", "")

	if got := String(cmd, "agent-id"); got != "  peer-agent-1  " {
		t.Fatalf("String = %q, want raw string", got)
	}
	if got := StringTrimmed(cmd, "agent-id"); got != "peer-agent-1" {
		t.Fatalf("StringTrimmed = %q, want peer-agent-1", got)
	}
	if got := StringTrimmedOrDefault(cmd, "empty-flag", "default-val"); got != "default-val" {
		t.Fatalf("StringTrimmedOrDefault = %q, want default-val", got)
	}
	if got := StringTrimmedOrDefault(cmd, "non-existent", "fallback"); got != "fallback" {
		t.Fatalf("StringTrimmedOrDefault missing = %q, want fallback", got)
	}

	reqVal, err := RequireStringTrimmed(cmd, "agent-id")
	if err != nil || reqVal != "peer-agent-1" {
		t.Fatalf("RequireStringTrimmed = %q, err = %v", reqVal, err)
	}

	_, err = RequireStringTrimmed(cmd, "empty-flag")
	if err == nil {
		t.Fatal("RequireStringTrimmed on empty string must return error")
	}
}

func TestFlagUtil_TypedGetters(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.Flags().Bool("once", true, "")
	cmd.Flags().Int("poll-seconds", 42, "")
	cmd.Flags().Int("zero-count", 0, "")
	cmd.Flags().Duration("timeout", 5*time.Second, "")
	cmd.Flags().StringSlice("tags", []string{" tag1 ", " ", "tag2"}, "")
	cmd.Flags().Float64("score", 98.6, "")

	if !Bool(cmd, "once") {
		t.Fatal("Bool once must be true")
	}
	if Bool(cmd, "missing") {
		t.Fatal("Bool missing must be false")
	}
	if Int(cmd, "poll-seconds") != 42 {
		t.Fatalf("Int = %d, want 42", Int(cmd, "poll-seconds"))
	}
	if IntOrDefault(cmd, "zero-count", 10) != 10 {
		t.Fatalf("IntOrDefault zero = %d, want 10", IntOrDefault(cmd, "zero-count", 10))
	}
	if IntOrDefault(cmd, "poll-seconds", 10) != 42 {
		t.Fatalf("IntOrDefault = %d, want 42", IntOrDefault(cmd, "poll-seconds", 10))
	}
	if Duration(cmd, "timeout") != 5*time.Second {
		t.Fatalf("Duration = %v, want 5s", Duration(cmd, "timeout"))
	}
	if DurationOrDefault(cmd, "missing-dur", time.Minute) != time.Minute {
		t.Fatalf("DurationOrDefault = %v, want 1m", DurationOrDefault(cmd, "missing-dur", time.Minute))
	}

	trimmedSlice := StringSliceTrimmed(cmd, "tags")
	if len(trimmedSlice) != 2 || trimmedSlice[0] != "tag1" || trimmedSlice[1] != "tag2" {
		t.Fatalf("StringSliceTrimmed = %v, want [tag1, tag2]", trimmedSlice)
	}

	if Float64(cmd, "score") != 98.6 {
		t.Fatalf("Float64 = %v, want 98.6", Float64(cmd, "score"))
	}
}

func TestFlagUtil_NilSafety(t *testing.T) {
	t.Parallel()

	if String(nil, "flag") != "" {
		t.Fatal("expected empty string on nil cmd")
	}
	if StringTrimmed(nil, "flag") != "" {
		t.Fatal("expected empty string on nil cmd")
	}
	if Bool(nil, "flag") {
		t.Fatal("expected false on nil cmd")
	}
	if Int(nil, "flag") != 0 {
		t.Fatal("expected 0 on nil cmd")
	}
	if Duration(nil, "flag") != 0 {
		t.Fatal("expected 0 on nil cmd")
	}
	if StringSlice(nil, "flag") != nil {
		t.Fatal("expected nil slice on nil cmd")
	}
	if IsChanged(nil, "flag") {
		t.Fatal("expected false on nil cmd")
	}
	if IsTrue(nil) {
		t.Fatal("expected false on nil flag")
	}
}
