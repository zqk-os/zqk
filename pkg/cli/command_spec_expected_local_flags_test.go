package cli

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestExpectedLocalFlagNamesFromCommandSpec_commonExcluded(t *testing.T) {
	spec := &CommandSpec{
		CommonFlags: true,
		Help: &HelpSpec{
			ExcludeFlags: DefaultCommonExcludedFlags(),
		},
	}
	got := ExpectedLocalFlagNamesFromCommandSpec(spec)
	if len(got) != 0 {
		t.Fatalf("full common exclusion should yield no common flags on cmd; got %v", got)
	}
}

func TestExpectedLocalFlagNamesFromCommandSpec_fieldsHarness(t *testing.T) {
	spec := &CommandSpec{
		FieldsHarnessFlags:     true,
		FieldsIncludeListKinds: false,
	}
	got := ExpectedLocalFlagNamesFromCommandSpec(spec)
	want := FieldsHarnessFlagNames(false)
	sort.Strings(want)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("(-want +got):\n%s", diff)
	}
}

func TestExpectedLocalFlagNamesFromCommandSpec_queryFlags(t *testing.T) {
	spec := &CommandSpec{
		QueryFlags: true,
	}
	got := ExpectedLocalFlagNamesFromCommandSpec(spec)
	want := QueryFlagNames()
	sort.Strings(want)
	sort.Strings(got)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("(-want +got):\n%s", diff)
	}
}

func TestExpectedLocalFlagNamesFromCommandSpec_withListHarness(t *testing.T) {
	spec := &CommandSpec{
		CommonFlags:      true,
		ListHarnessFlags: true,
		Help:             &HelpSpec{ExcludeFlags: DefaultCommonExcludedFlags()},
		Flags:            []FlagSpec{{Name: "built-in", Type: "bool", Description: "x"}},
	}
	got := ExpectedLocalFlagNamesFromCommandSpec(spec)
	want := append([]string{"built-in"}, ListHarnessFlagNames()...)
	sort.Strings(want)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("(-want +got):\n%s", diff)
	}
}
