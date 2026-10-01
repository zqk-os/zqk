package autofix

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

const autoFixRuleKind = objects.KindAutoFixRule

// RuleEntry is the in-memory representation of one auto_fix_rule object
// used for matching and placeholder substitution.
type RuleEntry struct {
	AppliesToKind            string
	ConditionCategory        string
	ConditionTier            int
	ConditionRule            string
	ConditionMessageContains string
	FixCommandTemplate       string
	Priority                 int
	Enabled                  bool
}

// AutoFixRuleLoader caches auto_fix_rule objects per project root and provides
// lookup for fix command templates by kind and condition. One List per project
// root (lazy); subsequent lookups use the cache so we do not List on every issue.
type AutoFixRuleLoader struct {
	mu     sync.RWMutex
	cache  map[string][]RuleEntry
	config *storage.OperationConfig
}

// NewAutoFixRuleLoader creates a loader with default list timeout.
func NewAutoFixRuleLoader() *AutoFixRuleLoader {
	return &AutoFixRuleLoader{
		cache: make(map[string][]RuleEntry),
		config: &storage.OperationConfig{
			Timeout: 10 * time.Second,
		},
	}
}

// GetRules loads auto_fix_rule objects for projectRoot from storage (once per project root),
// caches them, and returns the slice. Caller must hold at least rlock when using the returned slice
// if relying on cache consistency; we return a copy so callers can sort/filter without mutating cache.
func (l *AutoFixRuleLoader) GetRules(projectRoot string, storageProvider storage.ObjectStorageProvider) ([]RuleEntry, error) {
	if projectRoot == "" || storageProvider == nil {
		return nil, nil
	}

	l.mu.RLock()
	cached, ok := l.cache[projectRoot]
	l.mu.RUnlock()
	if ok {
		return cached, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	// Double-check after acquiring write lock
	if cached, ok = l.cache[projectRoot]; ok {
		return cached, nil
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	result, err := storage.ListWithConfig(storageProvider, l.config, secCtx, storageCtx, storage.ListFilter{
		Kind: autoFixRuleKind,
	})
	if err != nil {
		return nil, errfmt.Newf("list auto_fix_rule").Wrap(err)
	}
	if result == nil || len(result.Objects) == 0 {
		l.cache[projectRoot] = nil
		return nil, nil
	}

	entries := make([]RuleEntry, 0, len(result.Objects))
	for _, obj := range result.Objects {
		e := parseAutoFixRuleEntry(obj)
		if e.FixCommandTemplate == "" || e.AppliesToKind == "" {
			continue
		}
		entries = append(entries, e)
	}
	l.cache[projectRoot] = entries
	return entries, nil
}

func parseAutoFixRuleEntry(obj map[string]any) RuleEntry {
	e := RuleEntry{
		AppliesToKind:            getString(obj, "applies_to_kind"),
		ConditionCategory:        getString(obj, "condition_category"),
		ConditionRule:            getString(obj, "condition_rule"),
		ConditionMessageContains: getString(obj, "condition_message_contains"),
		FixCommandTemplate:       getString(obj, "fix_command_template"),
		Priority:                 100,
		Enabled:                  true,
	}
	e.ConditionTier = getInt(obj, "condition_tier")
	if p := getInt(obj, "priority"); p != 0 {
		e.Priority = p
	}
	if enabled, ok := obj[objects.FieldKeyEnabled].(bool); ok {
		e.Enabled = enabled
	}
	return e
}

func getString(obj map[string]any, key string) string {
	s, _ := obj[key].(string)
	return s
}

func getInt(obj map[string]any, key string) int {
	switch v := obj[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// GetFixCommand finds a matching enabled auto_fix_rule for the given kind and condition,
// substitutes placeholders in the template, and returns the command. Returns ("", false)
// if no rule matches or storage is unavailable. Lower priority value wins when multiple match.
func (l *AutoFixRuleLoader) GetFixCommand(
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	kind, category string,
	tier int,
	rule, message, objID, field string,
	objMap map[string]any,
) (string, bool) {
	rules, err := l.GetRules(projectRoot, storageProvider)
	if err != nil || len(rules) == 0 {
		return "", false
	}

	var matches []RuleEntry
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if r.AppliesToKind != kind {
			continue
		}
		if r.ConditionCategory != "" && r.ConditionCategory != category {
			continue
		}
		if r.ConditionTier != 0 && r.ConditionTier != tier {
			continue
		}
		if r.ConditionRule != "" && r.ConditionRule != rule {
			continue
		}
		if r.ConditionMessageContains != "" && !strings.Contains(message, r.ConditionMessageContains) {
			continue
		}
		matches = append(matches, r)
	}
	if len(matches) == 0 {
		return "", false
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].Priority < matches[j].Priority })
	template := matches[0].FixCommandTemplate
	cmd := SubstituteFixCommandPlaceholders(template, kind, category, tier, rule, message, objID, field)
	return cmd, cmd != ""
}

// SubstituteFixCommandPlaceholders replaces supported tokens in a fix template.
func SubstituteFixCommandPlaceholders(template, kind, category string, tier int, rule, message, objID, field string) string {
	repl := map[string]string{
		"{object_id}": objID,
		"{kind}":      kind,
		"{field}":     field,
		"{message}":   message,
		"{rule}":      rule,
		"{tier}":      fmt.Sprintf("%d", tier),
		"{category}":  category,
	}
	out := template
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

// InvalidateCache clears the cached rules for the given project root.
func (l *AutoFixRuleLoader) InvalidateCache(projectRoot string) {
	if projectRoot == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.cache, projectRoot)
}

var (
	globalAutoFixRuleLoader     *AutoFixRuleLoader
	globalAutoFixRuleLoaderOnce sync.Once
)

// GetAutoFixRuleLoader returns the process-wide auto_fix_rule loader (lazy init).
func GetAutoFixRuleLoader() *AutoFixRuleLoader {
	globalAutoFixRuleLoaderOnce.Do(func() {
		globalAutoFixRuleLoader = NewAutoFixRuleLoader()
	})
	return globalAutoFixRuleLoader
}
