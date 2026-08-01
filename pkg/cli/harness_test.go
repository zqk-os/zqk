package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

func TestListTraitSetFromExpanded(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expanded []string
		want     ListTraitSet
	}{
		{"empty", nil, ListTraitSet{}}, // no listable => CountOnly false
		{"listable only", []string{"listable"}, ListTraitSet{Listable: true, CountOnly: true}},
		{"read_only_group subset", []string{"listable", "readable", "formatable", "groupable", "filterable", "sortable", "searchable"}, ListTraitSet{Listable: true, Filterable: true, Sortable: true, Groupable: true, Searchable: true, Formatable: true, CountOnly: true}},
		{"unknown traits ignored", []string{"listable", "writable", "modifiable"}, ListTraitSet{Listable: true, CountOnly: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ListTraitSetFromExpanded(tt.expanded)
			if got != tt.want {
				t.Errorf("ListTraitSetFromExpanded() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRun_ResolvedSet_CountOnly(t *testing.T) {
	t.Parallel()
	cmd := cobra.Command{}
	AddListFlags(&cmd)
	_ = cmd.Flags().Set("count", "true")
	source := &staticListSource{
		items: []map[string]any{
			{objects.FieldKeyID: "a", objects.FieldKeyName: "Alpha"},
			{objects.FieldKeyID: "b", objects.FieldKeyName: "Beta"},
		},
	}
	cfg := &ListConfig{
		ResolvedSet: &ListTraitSet{Listable: true, CountOnly: true},
	}
	var buf bytes.Buffer
	err := Run(context.Background(), &cmd, source, cfg, &buf)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "2" {
		t.Errorf("count output = %q, want \"2\"", got)
	}
}

func TestRun_ResolvedSet_TableAndJSON(t *testing.T) {
	t.Parallel()
	source := &staticListSource{
		items: []map[string]any{
			{objects.FieldKeyID: "x", objects.FieldKeyName: "Ex", objects.FieldKeyEnabled: true},
		},
	}
	cfg := &ListConfig{
		ResolvedSet:  &ListTraitSet{Listable: true, Formatable: true},
		TableColumns: []string{"id", "name", "enabled"},
	}

	cmdTable := cobra.Command{}
	AddListFlags(&cmdTable)
	var bufTable bytes.Buffer
	err := Run(context.Background(), &cmdTable, source, cfg, &bufTable)
	if err != nil {
		t.Fatalf("Run table: %v", err)
	}
	out := bufTable.String()
	if !strings.Contains(out, "id") || !strings.Contains(out, "x") {
		t.Errorf("table output missing header or data: %s", out)
	}

	cmdJSON := cobra.Command{}
	AddListFlags(&cmdJSON)
	_ = cmdJSON.Flags().Set("format", "json")
	var bufJSON bytes.Buffer
	err = Run(context.Background(), &cmdJSON, source, cfg, &bufJSON)
	if err != nil {
		t.Fatalf("Run json: %v", err)
	}
	if !strings.Contains(bufJSON.String(), `"items"`) || !strings.Contains(bufJSON.String(), `"x"`) {
		t.Errorf("json output missing items or id: %s", bufJSON.String())
	}
}

func TestRun_ExpandTraits(t *testing.T) {
	t.Parallel()
	// Use a mock expander that returns a known list so we don't depend on pkg/objects
	expander := &mockTraitExpander{
		expanded: map[string][]string{
			"read_only_group": {"listable", "filterable", "sortable", "groupable", "formatable", "searchable"},
		},
	}
	cmd := cobra.Command{}
	AddListFlags(&cmd)
	source := &staticListSource{items: []map[string]any{{objects.FieldKeyID: "1"}}}
	cfg := &ListConfig{
		TraitGroups:   []string{"read_only_group"},
		TraitExpander: expander,
	}
	var buf bytes.Buffer
	err := Run(context.Background(), &cmd, source, cfg, &buf)
	if err != nil {
		t.Fatalf("Run with expander: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

func TestRun_Filter(t *testing.T) {
	t.Parallel()
	cmd := cobra.Command{}
	AddListFlags(&cmd)
	_ = cmd.Flags().Set("filter", "enabled=true")
	source := &staticListSource{
		items: []map[string]any{
			{objects.FieldKeyID: "a", objects.FieldKeyEnabled: true},
			{objects.FieldKeyID: "b", objects.FieldKeyEnabled: false},
		},
	}
	cfg := &ListConfig{
		ResolvedSet: &ListTraitSet{Listable: true, Filterable: true, Formatable: true},
	}
	var buf bytes.Buffer
	err := Run(context.Background(), &cmd, source, cfg, &buf)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Filter enabled=true should leave only the first row (id=a)
	out := buf.String()
	if !strings.Contains(out, "a") {
		t.Errorf("filtered output should contain a: %s", out)
	}
	var dataLines int
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) != emptyValue {
			dataLines++
		}
	}
	if dataLines < 2 {
		t.Errorf("expected at least header + 1 data row, got %d lines: %s", dataLines, out)
	}
}

type staticListSource struct {
	items []map[string]any
}

func (s *staticListSource) List(ctx context.Context, opts ListOptions) ([]map[string]any, int, error) {
	return s.items, len(s.items), nil
}

type mockTraitExpander struct {
	expanded map[string][]string
}

func (m *mockTraitExpander) ExpandTraits(traits []string) ([]string, error) {
	var out []string
	seen := make(map[string]bool)
	for _, t := range traits {
		if list, ok := m.expanded[t]; ok {
			for _, x := range list {
				if !seen[x] {
					seen[x] = true
					out = append(out, x)
				}
			}
		} else {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out, nil
}

// trigger tdd
