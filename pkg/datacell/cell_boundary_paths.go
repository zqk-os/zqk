package datacell

// CellCASPrimaryDir resolves .zqk/process/<entitySegment> for CAS-backed cells only through
// [CASEntityMembraneReadPaths] (CRIT-DATACELL-001 strict: packages outside pkg/datacell must not call
// [CASEntityPrimaryDir] directly — use this or other [MembraneReadPaths] methods).
func CellCASPrimaryDir(projectRoot, entitySegment string) string {
	return CASEntityMembraneReadPaths(projectRoot).CASEntityPrimaryDir(entitySegment)
}

// CellStreamOverlayKindDir resolves .zqk/state/stream_current/<kind> only through
// [StreamMembraneReadPaths] (same boundary rule as [CellCASPrimaryDir]).
func CellStreamOverlayKindDir(projectRoot, kind string) string {
	return StreamMembraneReadPaths(projectRoot).StreamCurrentKindDir(kind)
}
