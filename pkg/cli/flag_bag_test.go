package cli_test

import (
	"testing"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

func TestFlagBag_FirstErrorWins(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "t"}
	cmd.Flags().String("ok", "v", "")
	var f clipkg.FlagBag
	if got := f.String(cmd, "ok"); got != "v" {
		t.Fatalf("ok=%q", got)
	}
	_ = f.String(cmd, "missing")
	if f.Err() == nil {
		t.Fatal("expected error for missing flag")
	}
	first := f.Err()
	_ = f.Bool(cmd, "also-missing")
	if f.Err() != first {
		t.Fatalf("error mutated: %v vs %v", f.Err(), first)
	}
}

func TestFlagBag_AllTypes(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("str", "hello", "")
	cmd.Flags().Bool("bool", true, "")
	cmd.Flags().Int("int", 42, "")
	cmd.Flags().StringArray("strarr", []string{"a", "b"}, "")
	cmd.Flags().StringSlice("strslice", []string{"c", "d"}, "")
	cmd.Flags().Duration("dur", 5000000000, "")
	cmd.Flags().Float64("flt", 3.14, "")
	cmd.Flags().StringToString("strmap", map[string]string{"k": "v"}, "")

	var f clipkg.FlagBag
	if got := f.String(cmd, "str"); got != "hello" {
		t.Errorf("String() = %v, want hello", got)
	}
	if got := f.Bool(cmd, "bool"); !got {
		t.Errorf("Bool() = %v, want true", got)
	}
	if got := f.Int(cmd, "int"); got != 42 {
		t.Errorf("Int() = %v, want 42", got)
	}
	if got := f.StringArray(cmd, "strarr"); len(got) != 2 || got[0] != "a" {
		t.Errorf("StringArray() = %v", got)
	}
	if got := f.StringSlice(cmd, "strslice"); len(got) != 2 || got[0] != "c" {
		t.Errorf("StringSlice() = %v", got)
	}
	if got := f.Duration(cmd, "dur"); got.Seconds() != 5 {
		t.Errorf("Duration() = %v", got)
	}
	if got := f.Float64(cmd, "flt"); got != 3.14 {
		t.Errorf("Float64() = %v", got)
	}
	if got := f.StringToString(cmd, "strmap"); got["k"] != "v" {
		t.Errorf("StringToString() = %v", got)
	}
	if f.Err() != nil {
		t.Fatalf("unexpected error: %v", f.Err())
	}

	// Test nil command handling
	var fNil clipkg.FlagBag
	if got := fNil.String(nil, "any"); got != "" {
		t.Errorf("expected empty string on nil cmd, got %v", got)
	}
	if fNil.Err() == nil {
		t.Fatal("expected error on nil cmd")
	}
}

