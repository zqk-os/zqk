package hostload

import (
	"os"
	"time"

	"github.com/lanceman/zqk/pkg/brand"
)

const (
	// Idle below this → starve (leave ~one core of slack on a 10-core host).
	idleStarve = 0.08
	// Idle below this → tight.
	idleTight = 0.25
	// load1/ncpu at or above this → starve.
	loadStarve = 1.6
	// load1/ncpu at or above this → tight.
	loadTight = 1.0
)

const enabledFlagValue = "1"

// Snapshot is one reading of host CPU pressure. Negative IdleFraction or Load1 means unavailable.
type Snapshot struct {
	NCPU         int
	Load1        float64
	IdleFraction float64
	SampledAt    time.Time
}

// Level combines idle-tick delta and load average; the worse of the two wins.
func (s Snapshot) Level() Level {
	out := LevelHeadroom
	if s.IdleFraction >= 0 {
		switch {
		case s.IdleFraction < idleStarve:
			out = LevelStarve
		case s.IdleFraction < idleTight:
			out = LevelTight
		}
	}
	if s.NCPU > 0 && s.Load1 >= 0 {
		r := s.Load1 / float64(s.NCPU)
		var fromLoad Level
		switch {
		case r >= loadStarve:
			fromLoad = LevelStarve
		case r >= loadTight:
			fromLoad = LevelTight
		default:
			fromLoad = LevelHeadroom
		}
		if fromLoad > out {
			out = fromLoad
		}
	}
	return out
}

// Disabled reports that live host sensing is opted out (always headroom).
// Uses brand.EnvVar, not zqkenv.HostloadDisable: importing zqkenv runs
// agent_guard init and electrocutes foreground go test of this leaf package.
// TRACK: TDE-CEF-ZQKENV-AGENT-GUARD-SPLIT-001 — use zqkenv when: agent_guard
// init is a separate import and hostload tests no longer pull that panic.
func Disabled() bool {
	return os.Getenv(brand.EnvVar("HOSTLOAD_DISABLE")) == enabledFlagValue
}

// excludingSelf adds occupancy back into idle / subtracts it from load1 so a
// process can tell foreign pressure from its own bulk work.
func (s Snapshot) excludingSelf(selfUnits int) Snapshot {
	out := s
	if selfUnits <= 0 || s.NCPU <= 0 {
		return out
	}
	frac := float64(selfUnits) / float64(s.NCPU)
	if out.IdleFraction >= 0 {
		out.IdleFraction += frac
		if out.IdleFraction > 1 {
			out.IdleFraction = 1
		}
	}
	if out.Load1 >= 0 {
		out.Load1 -= float64(selfUnits)
		if out.Load1 < 0 {
			out.Load1 = 0
		}
	}
	return out
}

// CurrentLevel is the process-wide pressure reading. Headroom when disabled.
func CurrentLevel() Level {
	if Disabled() {
		return LevelHeadroom
	}
	return activeSampler().Snapshot().Level()
}

// CurrentLevelExcluding is CurrentLevel after treating selfUnits as our occupancy
// rather than foreign load (AV, other tenants).
func CurrentLevelExcluding(selfUnits int) Level {
	if Disabled() {
		return LevelHeadroom
	}
	return activeSampler().Snapshot().excludingSelf(selfUnits).Level()
}
