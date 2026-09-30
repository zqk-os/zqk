package hostload

// Level is how hard the host CPU is already working relative to capacity.
// Unknown/missing samples are LevelHeadroom — fail open so a sensor gap does not stall work.
type Level int

const (
	// LevelHeadroom: enough idle (or unknown) that static NumCPU budgets are fine.
	LevelHeadroom Level = iota
	// LevelTight: host is busy; cut fan-out in half.
	LevelTight
	// LevelStarve: host is CPU-bound; run the minimum (one) unit of bulk work.
	LevelStarve
)

// Scale returns n adjusted for host pressure. n<=1 is unchanged so progress never stops.
func Scale(n int, level Level) int {
	if n <= 1 {
		return n
	}
	switch level {
	case LevelStarve:
		return 1
	case LevelTight:
		half := n / 2
		if half < 1 {
			return 1
		}
		return half
	default:
		return n
	}
}

// OverBudget reports that inUse concurrent units exceed what Scale allows for capacity.
// Uses the raw host sample (includes this process). Prefer OverBudgetExcludingSelf for
// foreground bulk work that is supposed to use the cores it holds.
func OverBudget(inUse, capacity int) bool {
	if capacity <= 0 {
		return false
	}
	return inUse > Scale(capacity, CurrentLevel())
}

// OverBudgetExcludingSelf is OverBudget after removing already-held occupancy from
// the host sample. want is the occupancy being requested (typically held+1).
// do not bounce system-check slots
// because the check itself drove idle below idleTight.
func OverBudgetExcludingSelf(want, capacity int) bool {
	if capacity <= 0 {
		return false
	}
	self := want - 1
	if self < 0 {
		self = 0
	}
	return want > Scale(capacity, CurrentLevelExcluding(self))
}
