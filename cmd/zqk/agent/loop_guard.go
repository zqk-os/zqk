package agent

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// loop guard knobs + stagnation heuristic.

const (
	defaultMaxSyncLoops             = 100
	defaultMaxVerificationAttempts  = 3
	defaultMaxStagnantProgressTicks = 10
)

// LoopGuardConfig bounds sync-loop outer polls, verification retries, and stagnation.
type LoopGuardConfig struct {
	MaxSyncLoops             int
	MaxVerificationAttempts  int
	MaxStagnantProgressTicks int
}

// LoadLoopGuardConfig reads brand-prefixed env overrides (≤0 → defaults).
func LoadLoopGuardConfig() LoopGuardConfig {
	cfg := LoopGuardConfig{
		MaxSyncLoops:             zqkenv.Get(zqkenv.AgentSyncMaxLoops().Name()).IntOrDefault(defaultMaxSyncLoops),
		MaxVerificationAttempts:  zqkenv.Get(zqkenv.AgentMaxVerificationAttempts().Name()).IntOrDefault(defaultMaxVerificationAttempts),
		MaxStagnantProgressTicks: zqkenv.Get(zqkenv.AgentSyncMaxStagnantTicks().Name()).IntOrDefault(defaultMaxStagnantProgressTicks),
	}
	if cfg.MaxSyncLoops <= 0 {
		cfg.MaxSyncLoops = defaultMaxSyncLoops
	}
	if cfg.MaxVerificationAttempts <= 0 {
		cfg.MaxVerificationAttempts = defaultMaxVerificationAttempts
	}
	if cfg.MaxStagnantProgressTicks <= 0 {
		cfg.MaxStagnantProgressTicks = defaultMaxStagnantProgressTicks
	}
	return cfg
}

// stagnationGuard aborts when the task progress fingerprint is unchanged for Max ticks.
type stagnationGuard struct {
	max      int
	lastFP   string
	sameTick int
}

func newStagnationGuard(max int) *stagnationGuard {
	if max <= 0 {
		max = defaultMaxStagnantProgressTicks
	}
	return &stagnationGuard{max: max}
}

// Observe records fp; returns true when unchanged for more than max ticks (abort).
func (g *stagnationGuard) Observe(fp string) bool {
	if g == nil {
		return false
	}
	if fp == g.lastFP {
		g.sameTick++
	} else {
		g.lastFP = fp
		g.sameTick = 1
	}
	return g.sameTick > g.max
}

func verificationAttemptsOf(step map[string]any) int {
	if step == nil {
		return 0
	}
	raw, ok := step[objects.FieldKeyVerificationAttempts]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}

// taskProgressFingerprint captures status + step statuses/attempts so silent no-ops trip the guard.
func taskProgressFingerprint(task map[string]any) string {
	if task == nil {
		return ""
	}
	status, _ := task[objects.FieldKeyStatus].(string)
	var b strings.Builder
	b.WriteString(status)
	b.WriteByte('|')
	appendSteps := func(raw any) {
		list, ok := raw.([]any)
		if !ok {
			return
		}
		for _, item := range list {
			step, ok := item.(map[string]any)
			if !ok {
				continue
			}
			st, _ := step[objects.FieldKeyStatus].(string)
			name, _ := step[objects.FieldKeyName].(string)
			if name == "" {
				name, _ = step[objects.FieldKeyTitle].(string)
			}
			fmt.Fprintf(&b, "%s:%s:%d;", name, st, verificationAttemptsOf(step))
		}
	}
	if cv, ok := task[objects.FieldKeyCompletenessValidation]; ok {
		appendSteps(cv)
	} else if steps, ok := task[objects.FieldKeyTaskSteps]; ok {
		appendSteps(steps)
	}
	return b.String()
}
