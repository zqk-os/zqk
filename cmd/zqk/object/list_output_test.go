package object

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestParseTableColumns_usesProjectionFieldOrder(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().String("columns", "", "")

	f, _, h := parseTableColumns(cmd, nil, []string{"id", "priority_tier", "status"})
	if len(f) != 3 || f[0] != "id" || f[1] != "priority_tier" || f[2] != "status" {
		t.Fatalf("field order: got %v", f)
	}
	if len(h) != 3 {
		t.Fatalf("headers: got %d", len(h))
	}
}

func TestParseTableColumns_columnsFlagDefinesColumnsWithoutProjection(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().String("columns", "", "")
	_ = cmd.ParseFlags([]string{"--columns", "id:8,status:4"})

	f, w, h := parseTableColumns(cmd, nil, nil)
	if len(f) != 2 || f[0] != "id" || f[1] != "status" {
		t.Fatalf("without projection, --columns defines which columns: got %v", f)
	}
	if w[0] != 8 || w[1] != 4 {
		t.Fatalf("widths: %v", w)
	}
	if len(h) != 2 {
		t.Fatalf("headers: %v", h)
	}
}

func TestParseTableColumns_projectionMergesColumnsFlagWidths(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().String("columns", "", "")
	_ = cmd.ParseFlags([]string{"--columns", "id:36,priority_tier:15"})

	f, w, _ := parseTableColumns(cmd, nil, []string{"id", "status", "priority_tier"})
	if len(f) != 3 || f[0] != "id" || f[1] != "status" || f[2] != "priority_tier" {
		t.Fatalf("expected all projected fields, got %v", f)
	}
	if w[0] != 36 || w[2] != 15 {
		t.Fatalf("expected widths from --columns for id and priority_tier, got %v", w)
	}
	if w[1] != 10 {
		t.Fatalf("status should use default table width when not in --columns, got %d", w[1])
	}
}

func TestParseTableColumns_projectionMergesFieldsFlagWidths(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("fields", []string{"id:50", "status:10", "priority_tier:25"}, "")
	cmd.Flags().String("columns", "", "")

	f, w, _ := parseTableColumns(cmd, nil, []string{"id", "status", "priority_tier"})
	if len(f) != 3 || f[0] != "id" || f[1] != "status" || f[2] != "priority_tier" {
		t.Fatalf("expected all projected fields, got %v", f)
	}
	if w[0] != 50 || w[1] != 10 || w[2] != 25 {
		t.Fatalf("expected custom widths from --fields, got %v", w)
	}
}
