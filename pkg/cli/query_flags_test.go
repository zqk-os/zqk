package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNormalizeFieldsFlagValues_rejectsFlagLikeKey(t *testing.T) {
	_, err := normalizeFieldsFlagValues([]string{"-h"})
	if err == nil {
		t.Fatal("expected error for -h")
	}
	if !strings.Contains(err.Error(), "invalid --fields key") {
		t.Fatalf("got: %v", err)
	}
}

func TestNormalizeFieldsFlagValues_ok(t *testing.T) {
	got, err := normalizeFieldsFlagValues([]string{"id", "title, status"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
}

func TestFieldsFromCmd(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("fields", nil, "")
	if err := cmd.ParseFlags([]string{"--fields", "prompt_body"}); err != nil {
		t.Fatal(err)
	}
	got, err := FieldsFromCmd(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "prompt_body" {
		t.Fatalf("got %v", got)
	}
}
