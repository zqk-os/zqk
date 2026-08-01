package objects

import (
	"strings"
	"sync"
)

// Global projection slots: a fixed column order (bit i = global slot i) shared by all kinds.
// This is the "global" part of the hybrid layout; see kindProjectionExtensions for per-kind columns.
const globalProjectionSlotCount = 32

// Global bits and per-kind extension bits are separate uint32 values (up to 32 extension columns per kind).
// If a kind's extension table is longer than 32, use [HybridProjectionMask.Overflow] for the rest
// (rare; extend to a second ext word in a follow-up if needed).
const maxExtBits = 32

// HybridProjectionMask is a compact row in the N×M projection matrix: which global and kind-local
// columns to keep. Apply only to **materialized** maps (after CAS + runtime_delta + overlays).
type HybridProjectionMask struct {
	Global   uint32
	Ext      uint32
	Kind     string
	Overflow []string // top-level field names not in the global or kind extension registries
}

var globalSlotKeys = [globalProjectionSlotCount]string{
	0:  FieldKeyID,
	1:  FieldKeyKind,
	2:  FieldKeyTitle,
	3:  FieldKeyStatus,
	4:  FieldKeyDescription,
	5:  FieldKeyCreatedAt,
	6:  FieldKeyUpdatedAt,
	7:  FieldKeyCreatedBy,
	8:  FieldKeyUpdatedBy,
	9:  FieldKeySchemaVersion,
	10: FieldKeyMetadata,
	11: FieldKeyPriority,
	12: FieldKeyAccountID,
	13: FieldKeyDisplayName,
	14: FieldKeyName,
	15: FieldKeyType,
	16: FieldKeyEventType,
	17: FieldKeyContent,
	18: FieldKeyBody,
	19: FieldKeyLifecycleFilter,
	20: FieldKeyVersion,
	21: FieldKeyGroup,
	22: FieldKeyContext,
	23: FieldKeySource,
	24: FieldKeyGoalRefs,
	25: FieldKeyMilestoneRefs,
	26: FieldKeyWorkstreamRef,
	27: FieldKeyBacklogItemRefs,
	28: FieldKeyCriteriaRefs,
	29: FieldKeyRequirementRefs,
	30: FieldKeyTags,
	31: FieldKeyCompletedAt,
}

var (
	globalKeyToBit  map[string]int
	buildGlobalOnce sync.Once
)

func ensureGlobalProjectionIndex() {
	buildGlobalOnce.Do(func() {
		globalKeyToBit = make(map[string]int, globalProjectionSlotCount)
		for i := range globalProjectionSlotCount {
			k := globalSlotKeys[i]
			if k == emptyValue || k == "" {
				continue
			}
			globalKeyToBit[k] = i
		}
	})
}

// HybridMaskFromFields builds a projection mask from raw field names for the given kind.
// Names can be arbitrary top-level YAML keys; unknown names are stored in Overflow (still applied by [ProjectMapHybrid]).
func HybridMaskFromFields(kind string, fields []string) HybridProjectionMask {
	ensureGlobalProjectionIndex()
	kind = strings.TrimSpace(kind)
	mask := HybridProjectionMask{Kind: kind}
	overflowSeen := make(map[string]struct{})
	extIdx := kindExtensionIndex(kind)

	for _, raw := range fields {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if bit, ok := globalKeyToBit[key]; ok {
			mask.Global |= uint32(1) << bit
			continue
		}
		if extIdx != nil {
			if bi, ok := extIdx[key]; ok && bi >= 0 && bi < maxExtBits {
				mask.Ext |= uint32(1) << bi
				continue
			}
		}
		if _, dup := overflowSeen[key]; dup {
			continue
		}
		overflowSeen[key] = struct{}{}
		mask.Overflow = append(mask.Overflow, key)
	}
	return mask
}

// MergeHybridMask returns a mask that includes every column set in a or b (same kind assumed for Ext bits).
func MergeHybridMask(a, b HybridProjectionMask) HybridProjectionMask {
	kind := a.Kind
	if kind == "" {
		kind = b.Kind
	} else if b.Kind != "" && b.Kind != kind {
		kind = b.Kind
	}
	return HybridProjectionMask{
		Kind:     kind,
		Global:   a.Global | b.Global,
		Ext:      a.Ext | b.Ext,
		Overflow: appendUniqueStrings(a.Overflow, b.Overflow),
	}
}

// ListProjectionFieldNames unions explicit field names with sort-by for list responses (deduped, trimmed).
func ListProjectionFieldNames(fields []string, sortBy string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, f := range fields {
		add(f)
	}
	add(sortBy)
	return out
}

// HybridMaskForList builds a mask from List-style parameters: projected columns plus sort column when set.
func HybridMaskForList(kind string, fields []string, sortBy string) HybridProjectionMask {
	names := ListProjectionFieldNames(fields, sortBy)
	return HybridMaskFromFields(kind, names)
}

// ProjectMapHybrid copies selected top-level keys from src according to mask.
// An all-zero mask (no bits, no overflow keys) yields an empty map — callers should not invoke this when
// no projection was requested (use full src from storage instead).
func ProjectMapHybrid(src map[string]any, mask HybridProjectionMask) map[string]any {
	if src == nil {
		return nil
	}
	if mask.Global == 0 && mask.Ext == 0 && len(mask.Overflow) == 0 {
		return map[string]any{}
	}
	ensureGlobalProjectionIndex()
	out := make(map[string]any)
	for i := range globalProjectionSlotCount {
		if mask.Global&(uint32(1)<<i) == 0 {
			continue
		}
		key := globalSlotKeys[i]
		if key == "" {
			continue
		}
		if v, ok := src[key]; ok {
			out[key] = v
		}
	}
	extKeys := kindExtensionKeys(mask.Kind)
	for i := 0; i < len(extKeys) && i < maxExtBits; i++ {
		if mask.Ext&(uint32(1)<<i) == 0 {
			continue
		}
		key := extKeys[i]
		if key == "" {
			continue
		}
		if v, ok := src[key]; ok {
			out[key] = v
		}
	}
	for _, key := range mask.Overflow {
		if v, ok := src[key]; ok {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func appendUniqueStrings(a, b []string) []string {
	if len(b) == 0 {
		return append([]string{}, a...)
	}
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, s := range b {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
