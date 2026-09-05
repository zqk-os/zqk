package objects

import "testing"

func TestMergeResolvedOverlayChildWins(t *testing.T) {
	t.Parallel()
	dst := &Spec{
		ResolvedFields: map[string]any{FieldKeyClaimedBy: "child"},
		ResolvedTraits: []string{"occupiable"},
	}
	src := &Spec{
		ResolvedFields: map[string]any{
			FieldKeyClaimedBy: "mixin",
			FieldKeyClaimedAt: "ts",
		},
		ResolvedTraits: []string{"occupiable", "completable"},
		StorageProfile: "cas_entity",
	}
	mergeResolvedOverlay(dst, src)
	if got := dst.ResolvedFields[FieldKeyClaimedBy]; got != "child" {
		t.Fatalf("claimed_by=%v want child (leaf wins)", got)
	}
	if _, ok := dst.ResolvedFields[FieldKeyClaimedAt]; !ok {
		t.Fatal("claimed_at should overlay from mixin")
	}
	hasCompletable := false
	for _, tr := range dst.ResolvedTraits {
		if tr == "completable" {
			hasCompletable = true
		}
	}
	if !hasCompletable {
		t.Fatalf("traits=%v missing completable from mixin", dst.ResolvedTraits)
	}
	if dst.StorageProfile != "cas_entity" {
		t.Fatalf("storage_profile=%q want fill from mixin when empty", dst.StorageProfile)
	}
}
