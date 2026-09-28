package workpack

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestKindsArePlanningAndVerification(t *testing.T) {
	got := Kinds()
	want := []string{
		objects.KindGoal,
		objects.KindRequirement,
		objects.KindCriteria,
		objects.KindTestCase,
	}
	if len(got) != len(want) {
		t.Fatalf("kinds %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kinds[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestEnableRecordsTheLink(t *testing.T) {
	enabled = false
	Enable()
	if !Enabled() {
		t.Fatal("Enable did not record the link")
	}
}

func TestRegisterRequiresARoot(t *testing.T) {
	linkedRoot = false
	Register(nil)
	if LinkedRoot() {
		t.Fatal("nil root counted as linked")
	}
	Register(&cobra.Command{Use: "zqk"})
	if !LinkedRoot() {
		t.Fatal("Register did not record the root")
	}
}

func TestBuilderCountMatchesKinds(t *testing.T) {
	if BuilderCount() != len(Kinds()) {
		t.Fatalf("builders %d kinds %d", BuilderCount(), len(Kinds()))
	}
}
