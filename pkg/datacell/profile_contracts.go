package datacell

// ProfileContract captures required behavior for each storage profile.
//
// Authoritative wire values: [StorageProfile], [KnownStorageProfiles], [ParseStorageProfile] (spec load:
// pkg/objects/spec_loader.go). Broader architecture: docs/architecture/DATA_CELL_MODEL.md,
// docs/architecture/DATA_CELL_RUNTIME_ORGANISM.md; stream/CAS stewardship alignment is checked in
// objects.TestSpecIndexStorageProfileMatchesHighVolumeStreamKinds and operator drift via
// `zqk system data-cells` (see cmd/zqk/system/data_cells.go).
// Stream-vs-high-volume alignment with docs/process/_internal/configs/high_volume_kinds.yaml is enforced
// by objects.TestSpecIndexStorageProfileMatchesHighVolumeStreamKinds (spec index vs canonical list).
// Invalid storage_profile values fail spec load; materialized spec index generation uses
// objects.BuildSpecIndexFromSpecsDirStrict so broken specs cannot be silently skipped from the JSON snapshot.
// Operators also see drift via zqk system data-cells (table / --json-envelope stream_stewardship_drift).
// This keeps contract semantics in one place for CLI discovery and tests.
type ProfileContract struct {
	Profile StorageProfile `json:"profile"`
	// WriteMode describes how writes are expected to persist for this profile.
	WriteMode string `json:"write_mode"`
	// ReadMode describes how readers typically resolve current state.
	ReadMode string `json:"read_mode"`
	// IntegrityMode describes the integrity mechanism expected for the profile.
	IntegrityMode string `json:"integrity_mode"`
	// MigrationMode describes the expected migration posture for the profile.
	MigrationMode string `json:"migration_mode"`
	// MembraneTransport names how reads resolve at the cell boundary (CRIT-DATACELL-002).
	MembraneTransport string `json:"membrane_transport"`
	// MembranePolicy names migration gating and policy visibility at the membrane (CRIT-DATACELL-002).
	MembranePolicy string `json:"membrane_policy"`
}

var profileContracts = map[StorageProfile]ProfileContract{
	ProfileCASEntity: {
		Profile:           ProfileCASEntity,
		WriteMode:         "cas_object_write",
		ReadMode:          "cas_snapshot_read",
		IntegrityMode:     "hash_registry_and_spec_validation",
		MigrationMode:     "id_preserving_copy_with_hash_reconcile",
		MembraneTransport: "cas_paths_via_membrane_read_paths",
		MembranePolicy:    "hash_spec_and_instance_validation_before_cross_profile_export",
	},
	ProfileLightFile: {
		Profile:           ProfileLightFile,
		WriteMode:         "single_file_or_small_set_write",
		ReadMode:          "direct_file_read",
		IntegrityMode:     "schema_and_shape_validation",
		MigrationMode:     "path_move_or_format_upgrade",
		MembraneTransport: "light_file_paths_via_membrane_read_paths",
		MembranePolicy:    "schema_and_hook_profile_checks_before_move",
	},
	ProfileStream: {
		Profile:           ProfileStream,
		WriteMode:         "append_segment_and_registry_update",
		ReadMode:          "registry_and_runtime_overlay_read",
		IntegrityMode:     "segment_registry_and_stewardship_checks",
		MigrationMode:     "segment_rebuild_or_profile_transform",
		MembraneTransport: "stream_paths_via_membrane_read_paths",
		MembranePolicy:    "segment_stewardship_gates_before_profile_transform",
	},
}

// ContractForProfile returns the contract for a known profile.
func ContractForProfile(profile StorageProfile) (ProfileContract, bool) {
	contract, ok := profileContracts[profile]
	return contract, ok
}
