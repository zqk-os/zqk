package datacell

import "testing"

// TestProfileContract_migrationChain_lightFileToCASToStream locks the intended **operator escalation**
// order for storage profiles: local / light surfaces → hash-addressed process objects → append streams.
//
// This does **not** execute physical data migration (CAS rewrite, segment rebuild, path moves) — those
// live under pkg/storage and stream tooling. Here we only assert that each profile exposes a distinct
// migration posture and write path so swapping storage_profile in object_specs implies different
// stewardship expectations (see profile_contracts.go and DATA_CELL_MODEL.md *Storage profiles in action*).
func TestProfileContract_migrationChain_lightFileToCASToStream(t *testing.T) {
	t.Parallel()
	chain := OperatorProfileMigrationChain()
	seen := make(map[string]StorageProfile, len(chain))
	for _, p := range chain {
		c, ok := ContractForProfile(p)
		if !ok {
			t.Fatalf("ContractForProfile(%s): expected contract", p)
		}
		if prev, dup := seen[c.MigrationMode]; dup {
			t.Fatalf("MigrationMode %q used by both %s and %s — migration postures must stay distinct",
				c.MigrationMode, prev, p)
		}
		seen[c.MigrationMode] = p
	}
	for i := 1; i < len(chain); i++ {
		from, to := chain[i-1], chain[i]
		cFrom, ok1 := ContractForProfile(from)
		cTo, ok2 := ContractForProfile(to)
		if !ok1 || !ok2 {
			t.Fatalf("missing contract for %s or %s", from, to)
		}
		if cFrom.MigrationMode == cTo.MigrationMode {
			t.Fatalf("step %s -> %s: MigrationMode unchanged %q", from, to, cFrom.MigrationMode)
		}
		if cFrom.WriteMode == cTo.WriteMode {
			t.Fatalf("step %s -> %s: WriteMode unchanged %q (profiles should imply different write models)",
				from, to, cFrom.WriteMode)
		}
	}
}

// TestOperationalEnvelope_profilesAreDistinctAcrossChain ensures v1 envelopes differ by profile so
// discovery / scheduler dry-run surfaces are not aliased when a kind moves along the migration story.
func TestOperationalEnvelope_profilesAreDistinctAcrossChain(t *testing.T) {
	t.Parallel()
	chain := OperatorProfileMigrationChain()
	var prevCompact string
	for _, p := range chain {
		env, ok := OperationalEnvelopeForProfile(p)
		if !ok {
			t.Fatalf("OperationalEnvelopeForProfile(%s)", p)
		}
		compact := env.CompactSummary()
		if compact == "" {
			t.Fatalf("empty CompactSummary for %s", p)
		}
		if prevCompact != "" && compact == prevCompact {
			t.Fatalf("OperationalEnvelope CompactSummary collided for profile %s (same string as previous step)", p)
		}
		prevCompact = compact
	}
}
