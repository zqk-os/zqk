package context

import (
	"testing"
)

func TestProfileLoader_ApplyNamedProfile_unknownProfile(t *testing.T) {
	pl := NewProfileLoader("")
	ctx := &Context{}
	err := pl.ApplyNamedProfile(ctx, "definitely-not-a-real-profile-xyz")
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestApplyProfileSpecToContext_storage(t *testing.T) {
	ctx := &Context{}
	ApplyProfileSpecToContext(ctx, &ProfileSpec{
		ResolvedFormat:  "json",
		ResolvedVerbose: true,
		ResolvedQuiet:   false,
		ResolvedStorage: map[string]any{
			"max_page_size":     42,
			"default_page_size": 10,
			"enable_grouping":   true,
			"max_group_size":    7,
		},
	})
	if ctx.Format != "json" || !ctx.Verbose || ctx.Quiet {
		t.Fatalf("unexpected format/verbose/quiet: %#v", ctx)
	}
	if ctx.StorageMaxPageSize != 42 || ctx.StorageDefaultPageSize != 10 || !ctx.StorageEnableGrouping || ctx.StorageMaxGroupSize != 7 {
		t.Fatalf("unexpected storage: max=%d def=%d group=%v maxg=%d",
			ctx.StorageMaxPageSize, ctx.StorageDefaultPageSize, ctx.StorageEnableGrouping, ctx.StorageMaxGroupSize)
	}
}
