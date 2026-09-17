package pm

import (
	"github.com/lanceman/zqk/pkg/pm"
)

// DefaultGenesisParentHash is the standard root genesis hash for legacy provenance blocks.
const DefaultGenesisParentHash = pm.DefaultGenesisParentHash

// InvariantPredicateFunc executes domain invariant checks against target objects.
type InvariantPredicateFunc = pm.InvariantPredicateFunc

// BacklogItem models an atomic unit of execution in the cellular PM domain.
type BacklogItem = pm.BacklogItem

// InvariantGate models a verifiable quality or security barrier for lifecycle transitions.
type InvariantGate = pm.InvariantGate

// Epic represents a cellular objective and enclave scope container.
type Epic = pm.Epic

// ADR models an Architectural Decision Record in the cellular schema.
type ADR = pm.ADR

// LegacyBacklogItem models the pre-cellular schema for legacy BacklogItems.
type LegacyBacklogItem = pm.LegacyBacklogItem

// NewBacklogItem constructs a new BacklogItem with valid cellular BaseObject and Lifecycle.
var NewBacklogItem = pm.NewBacklogItem

// NewInvariantGate constructs a new InvariantGate entity.
var NewInvariantGate = pm.NewInvariantGate

// NewProvenanceInvariantGate constructs an InvariantGate verifying cryptographic provenance.
var NewProvenanceInvariantGate = pm.NewProvenanceInvariantGate

// NewEpic constructs a new Epic entity with an automatic promotion invariant gate.
var NewEpic = pm.NewEpic

// NewADR constructs a new ADR entity.
var NewADR = pm.NewADR

// MigrateLegacyBacklogItem ingests raw YAML or JSON data of a legacy BacklogItem.
var MigrateLegacyBacklogItem = pm.MigrateLegacyBacklogItem

// ConvertLegacyBacklogItem converts a decoded LegacyBacklogItem struct into a cellular BacklogItem.
var ConvertLegacyBacklogItem = pm.ConvertLegacyBacklogItem

// ExtractCellID extracts a valid cellular cell name from a legacy namespace identifier.
var ExtractCellID = pm.ExtractCellID
