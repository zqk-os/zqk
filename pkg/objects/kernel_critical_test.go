package objects

import (
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
)

func TestEffectiveKernelCritical_inference(t *testing.T) {
	t.Parallel()
	cas := &Spec{Ontology: "x", StorageProfile: string(datacell.ProfileCASEntity)}
	if !EffectiveKernelCritical(cas) {
		t.Fatal("cas_entity should default critical")
	}
	stream := &Spec{Ontology: "y", StorageProfile: string(datacell.ProfileStream)}
	if EffectiveKernelCritical(stream) {
		t.Fatal("stream should default non-critical")
	}
	f := false
	optOut := &Spec{Ontology: "z", StorageProfile: string(datacell.ProfileCASEntity), KernelCritical: &f}
	if EffectiveKernelCritical(optOut) {
		t.Fatal("explicit false must win")
	}
	tr := true
	force := &Spec{Ontology: "s", StorageProfile: string(datacell.ProfileStream), KernelCritical: &tr}
	if !EffectiveKernelCritical(force) {
		t.Fatal("explicit true must win over stream inference")
	}
}

func TestSpecKindSummary_EffectiveKernelCritical_nilUsesProfile(t *testing.T) {
	t.Parallel()
	ks := SpecKindSummary{Kind: "backlog_item", StorageProfile: string(datacell.ProfileCASEntity)}
	if !ks.EffectiveKernelCritical() {
		t.Fatal("want critical from cas_entity when kernel_critical omitted in index")
	}
	ks2 := SpecKindSummary{Kind: "audit_event", StorageProfile: string(datacell.ProfileStream)}
	if ks2.EffectiveKernelCritical() {
		t.Fatal("want non-critical from stream")
	}
}
