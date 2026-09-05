package storage

// Version constants for storage artifacts and config (Phase B – single place for version literals).
// See docs/architecture/CONSTANTS_AND_DRY_INVENTORY_PLAN.md Phase B.

const (
	// CASIndexFormatVersion is the version written to filecas.IDIndex.Version (CAS kind index JSON).
	CASIndexFormatVersion = "1.0"
	// BlockingCheckConfigVersion is the version for default blocking check config (blocking_check_config.yaml).
	BlockingCheckConfigVersion = "1.0.0"
)
