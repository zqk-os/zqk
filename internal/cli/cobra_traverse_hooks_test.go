package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestTraversePersistentPreRunOrder documents cobra behavior when EnableTraverseRunHooks is true:
// PersistentPreRunE runs for each ancestor from root toward the invoked leaf (see spf13/cobra execute).
// zqk relies on this so nested commands receive root initialization and group-level kind validation.
func TestTraversePersistentPreRunOrder(t *testing.T) {
	prev := cobra.EnableTraverseRunHooks
	cobra.EnableTraverseRunHooks = true
	defer func() { cobra.EnableTraverseRunHooks = prev }()

	var seen []string
	root := &cobra.Command{Use: "root"}
	root.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		seen = append(seen, "root-pre")
		return nil
	}
	group := &cobra.Command{Use: "group"}
	group.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		seen = append(seen, "group-pre")
		return nil
	}
	leaf := &cobra.Command{
		Use: "leaf",
		RunE: func(*cobra.Command, []string) error {
			seen = append(seen, "leaf-run")
			return nil
		},
	}
	group.AddCommand(leaf)
	root.AddCommand(group)
	root.SetArgs([]string{"group", "leaf"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []string{"root-pre", "group-pre", "leaf-run"}
	if len(seen) != len(want) {
		t.Fatalf("got %v (%d steps), want %v", seen, len(seen), want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("step %d: got %q want %q (full %v)", i, seen[i], want[i], seen)
		}
	}
}
