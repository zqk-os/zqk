package dna

import (
	"strings"

	strutil "github.com/zqk-os/zqk/pkg/stringutil"
)

// Foundational biological traits enforced by the Cellular OS microkernel.
const (
	// TraitAuditable enforces cryptographic provenance, agent attribution, and causal lineage sealing on mutations.
	TraitAuditable = "auditable"
	// TraitLifecycle enforces state machine progression, permissible transitions, and invariant-guarded hops.
	TraitLifecycle = "lifecycle"
	// TraitStreamable identifies entities emitting delta mutation events.
	TraitStreamable = "streamable"
	// TraitOccupiable identifies entities requiring atomic agent concurrency leasing.
	TraitOccupiable = "occupiable"
	// TraitTraceable identifies entities requiring upward causal lineage to root intent.
	TraitTraceable = "traceable"
	// TraitAttestable identifies entities requiring criteria verification before equilibrium.
	TraitAttestable = "attestable"
	// TraitApoptotic identifies entities subject to automated quarantine and tombstoning.
	TraitApoptotic = "apoptotic"
)

// VetTraitInvariants evaluates native behavioral invariants demanded by an entity's declared traits.
func VetTraitInvariants(schema *MetaSchema, obj any, targetPhase MacroPhase, targetPlane Plane) error {
	if schema == nil || obj == nil {
		return nil
	}

	for _, trait := range schema.Traits {
		normalized := strings.ToLower(strings.TrimSpace(trait))
		switch normalized {
		case TraitAuditable:
			if err := vetAuditableTrait(obj, targetPlane); err != nil {
				return err
			}
		}
	}

	return nil
}

func vetAuditableTrait(obj any, targetPlane Plane) error {
	if targetPlane == PlanePromoted || targetPlane == PlaneStaged {
		if aud, ok := obj.(interface{ GetProvenance() Provenance }); ok {
			prov := aud.GetProvenance()
			if err := strutil.ErrorOnBlank(prov.Actor(), "trait invariant violation (auditable): target plane %s requires non-empty AgentURN or AgentID", targetPlane); err != nil {
				return err
			}
			if targetPlane == PlanePromoted {
				if err := strutil.ErrorOnBlank(prov.ParentHash, "trait invariant violation (auditable): target plane PlanePromoted requires causal ParentHash"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
