package objects

import "testing"

func TestWithoutKindsDropsOwned(t *testing.T) {
	got := WithoutKinds([]string{"goal", "account", "requirement"}, []string{"goal", "requirement"})
	if len(got) != 1 || got[0] != "account" {
		t.Fatalf("without kinds: %v", got)
	}
	same := []string{"account"}
	if got := WithoutKinds(same, nil); len(got) != 1 || got[0] != "account" {
		t.Fatalf("empty owned changed the list: %v", got)
	}
}

func TestSetPackOwnedKindsRoundTrip(t *testing.T) {
	defer SetPackOwnedKinds(nil)
	SetPackOwnedKinds([]string{"goal", "requirement", "criteria", "test_case"})
	got := PackOwnedKinds()
	want := []string{"goal", "requirement", "criteria", "test_case"}
	if len(got) != len(want) {
		t.Fatalf("pack kinds %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pack kinds[%d]=%q", i, got[i])
		}
	}
}
