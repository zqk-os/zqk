package datacell

import "fmt"

// OperatorProfileMigrationChain returns the canonical operator escalation order for cross-profile moves:
// local/light-file surfaces → CAS-backed entity storage → append streams for high-volume kinds.
//
// Physical migration (CAS rewrites, segment rebuilds, path moves) stays in pkg/storage / stream tooling;
// this ordering matches profile contracts (MigrationMode / WriteMode) and is documented for operators in
// docs/architecture/DATA_CELL_CROSS_PROFILE_MIGRATION_RUNBOOK.md.
func OperatorProfileMigrationChain() []StorageProfile {
	// Canonical escalation path
	return []StorageProfile{
		ProfileLightFile, // 1. Local / Light File (Surface)
		ProfileCASEntity, // 2. CAS-backed Entity
		ProfileStream,    // 3. High-volume Stream
	}
}

// ValidateMigrationOrder checks that the defined chain matches contract MigrationModes.
func ValidateMigrationOrder() error {
	chain := OperatorProfileMigrationChain()
	for i := 0; i < len(chain)-1; i++ {
		from, _ := ContractForProfile(chain[i])
		to, _ := ContractForProfile(chain[i+1])
		if from.MigrationMode == to.MigrationMode {
			return fmt.Errorf("migration step %s -> %s: MigrationMode unchanged (%s)", chain[i], chain[i+1], from.MigrationMode)
		}
	}
	return nil
}
