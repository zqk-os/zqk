package datacell

import (
	"context"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// CellHandle is a thin facade: one project root, storage profile, and [CellCoordinator] for stewardship
// enqueue plus read-path routing via [MembraneReadPathsForProfile]. Routine object CRUD stays on the existing
// object/list storage pipeline; use this handle for coordinator ops and membrane paths together.
type CellHandle struct {
	Root    string
	Profile StorageProfile
	Coord   CellCoordinator
}

// NewCellHandle returns a handle for profile. coordinator may be nil (uses [NoopCellCoordinator] via [MinimalMembrane] paths only).
func NewCellHandle(projectRoot string, profile StorageProfile, coord CellCoordinator) (*CellHandle, error) {
	if projectRoot == "" {
		return nil, errfmt.Errorf("cell handle: project root required")
	}
	if !profile.IsKnown() {
		return nil, errfmt.Errorf("cell handle: unknown storage_profile %q", profile)
	}
	if coord == nil {
		coord = NoopCellCoordinator{}
	}
	return &CellHandle{Root: projectRoot, Profile: profile, Coord: coord}, nil
}

// MembraneReadPaths returns read-path adapters for this handle's profile (stream uses the handle's coordinator).
func (h *CellHandle) MembraneReadPaths() MembraneReadPaths {
	if h == nil {
		return MembraneReadPaths{}
	}
	return MembraneReadPathsForProfile(h.Root, h.Profile, h.Coord)
}

// EnqueueMaintenance forwards to the coordinator ([REQ-DATACELL-001] enqueue surface).
func (h *CellHandle) EnqueueMaintenance(ctx context.Context, op MaintenanceOp) error {
	if h == nil {
		return nil
	}
	return h.Coord.Enqueue(ctx, op)
}

// MembraneReadPathsForProfile returns the v1 [MembraneReadPaths] constructor for profile. coord is used
// only for [ProfileStream] ([StreamMembraneReadPathsWithCoordinator]).
func MembraneReadPathsForProfile(projectRoot string, profile StorageProfile, coord CellCoordinator) MembraneReadPaths {
	switch profile {
	case ProfileLightFile:
		return RuntimeOrganismMembraneReadPaths(projectRoot)
	case ProfileStream:
		return StreamMembraneReadPathsWithCoordinator(projectRoot, coord)
	case ProfileCASEntity:
		return CASEntityMembraneReadPaths(projectRoot)
	default:
		return MembraneReadPaths{}
	}
}
