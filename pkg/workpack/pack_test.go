package workpack

import (
	"path/filepath"
	"strings"
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
		objects.KindMission,
		objects.KindVision,
		objects.KindStrategicContext,
		objects.KindStrategicPlan,
		objects.KindRoadmap,
		objects.KindMilestone,
		objects.KindEpic,
		objects.KindBacklogItem,
		objects.KindWorkstream,
		objects.KindPriorityPlan,
		objects.KindRiskBlocker,
		objects.KindWorkInterval,
		objects.KindWorkUnit,
		objects.KindOccupancy,
		objects.KindRemainingOpen,
		objects.KindWorkstreamTransition,
		objects.KindImportantDate,
		objects.KindTechnicalDebt,
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

func TestVerifyRecordsEachPlanningSpec(t *testing.T) {
	enabled = false
	verifiedSpecs = nil
	Enable()
	if !Enabled() {
		t.Fatal(VerifyError())
	}
	for _, kind := range Kinds() {
		got, ok := VerifiedSpec(kind)
		wantSuffix := filepath.Join("packs", "work", "specs", kind+".yaml")
		if !ok || !strings.HasSuffix(got, wantSuffix) {
			t.Fatalf("%s spec %q", kind, got)
		}
	}
}

func TestVerifyRejectsAKindWithoutASpec(t *testing.T) {
	_, err := verifyKindFiles(t.TempDir(), []string{"goal"}, "specs", "lifecycles")
	if err == nil {
		t.Fatal("missing spec was accepted")
	}
}

func TestBuilderCountMatchesKinds(t *testing.T) {
	if BuilderCount() != len(Kinds()) {
		t.Fatalf("builders %d kinds %d", BuilderCount(), len(Kinds()))
	}
}
