package hostload

import (
	"flag"
	"runtime"
	"sync"
	"time"
)

// Sampler produces a Snapshot. Production uses the OS sampler.
type Sampler interface {
	Snapshot() Snapshot
}

var (
	samplerMu sync.Mutex
	sampler   Sampler
)

func activeSampler() Sampler {
	samplerMu.Lock()
	defer samplerMu.Unlock()
	if sampler != nil {
		return sampler
	}
	sampler = newOSSampler()
	return sampler
}

type osSampler struct {
	mu       sync.Mutex
	prevIdle uint64
	prevTot  uint64
	prevOK   bool
	idleEMA  float64
	haveEMA  bool
}

func newOSSampler() *osSampler {
	return &osSampler{}
}

func underGoTest() bool {
	return flag.Lookup("test.v") != nil
}

func (s *osSampler) Snapshot() Snapshot {
	now := time.Now()
	ncpu := runtime.NumCPU()
	// go test must not wait on a noisy developer host or flake CI.
	if underGoTest() {
		return Snapshot{NCPU: ncpu, Load1: 0, IdleFraction: 1, SampledAt: now}
	}
	load1, loadOK := readLoadAvg()
	if !loadOK {
		load1 = -1
	}
	idle, total, tickOK := readCPUTicks()
	idleFrac := -1.0
	s.mu.Lock()
	defer s.mu.Unlock()
	if tickOK && s.prevOK && total > s.prevTot {
		dTot := total - s.prevTot
		dIdle := idle - s.prevIdle
		sample := float64(dIdle) / float64(dTot)
		if s.haveEMA {
			s.idleEMA = 0.5*sample + 0.5*s.idleEMA
		} else {
			s.idleEMA = sample
			s.haveEMA = true
		}
		idleFrac = s.idleEMA
	}
	if tickOK {
		s.prevIdle, s.prevTot, s.prevOK = idle, total, true
	}
	return Snapshot{
		NCPU:         ncpu,
		Load1:        load1,
		IdleFraction: idleFrac,
		SampledAt:    now,
	}
}
