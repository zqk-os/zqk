package objects

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/kindnames"
)

// CLI shortcut tokens that are object subcommand groups (not schema kinds by themselves).
// Used for tip text when resolution still fails; synonyms in pkg/kindsynonyms cover the happy path.
// TRACK: BLI-1785907446155489000-2d7f745b
var cliObjectShortcutTips = map[string]struct {
	CanonicalKind string
	ShortcutCmd   string
}{
	"splan":   {kindnames.StrategicPlan, "splan"},
	"pplan":   {kindnames.PriorityPlan, "pplan"},
	"wstrans": {kindnames.WorkstreamTransition, "wstrans"},
	"evoman":  {kindnames.EvolutionManagement, "evoman"},
	"draft":   {"", "draft"}, // draft is a verb group, not a kind synonym
}

// ResolveAndValidateKindForProject trims rawArg, resolves synonym aliases via GetCanonicalKind,
// and when a materialized spec index exists for projectRoot, rejects kinds not present in the index
// before expensive storage work.
func ResolveAndValidateKindForProject(projectRoot, rawArg string) (string, error) {
	raw := strings.TrimSpace(rawArg)
	if raw == "" {
		return "", errfmt.Errorf("kind argument is empty")
	}
	canonical := GetCanonicalKind(raw)
	if canonical == "" {
		return "", errfmt.Errorf("invalid kind %q", rawArg)
	}
	if idx := TryLoadSpecIndexForProjectRoot(projectRoot); idx != nil {
		if _, ok := idx.GetKindSummary(canonical); !ok {
			return "", unknownKindError(raw, canonical)
		}
	}
	return canonical, nil
}

func unknownKindError(raw, canonical string) error {
	key := strings.ToLower(strings.TrimSpace(raw))
	if tip, ok := cliObjectShortcutTips[key]; ok {
		if tip.CanonicalKind != "" {
			return errfmt.Errorf(
				"unknown kind %q: not in this project's object spec index. Tip: use %q object %s list (shortcut) or %q object list %s (canonical kind)",
				raw, "zqk", tip.ShortcutCmd, "zqk", tip.CanonicalKind)
		}
		return errfmt.Errorf(
			"unknown kind %q: %q is an object subcommand group, not a schema kind. Tip: use %q object %s …",
			raw, tip.ShortcutCmd, "zqk", tip.ShortcutCmd)
	}
	if strings.EqualFold(raw, canonical) {
		return errfmt.Errorf("unknown kind %q: not in this project's object spec index (update specs or use a registered kind name)", canonical)
	}
	return errfmt.Errorf("unknown kind %q (from %q): not in this project's object spec index (update specs or use a registered kind name)", canonical, raw)
}

// ResolveAndValidateKindsCommaSeparated splits a comma-separated kind argument, validates each
// segment with [ResolveAndValidateKindForProject], and returns canonical names in order.
func ResolveAndValidateKindsCommaSeparated(projectRoot, kindArg string) ([]string, error) {
	kindStrs := strings.Split(kindArg, ",")
	kinds := make([]string, 0, len(kindStrs))
	for _, k := range kindStrs {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		canonical, err := ResolveAndValidateKindForProject(projectRoot, k)
		if err != nil {
			return nil, err
		}
		kinds = append(kinds, canonical)
	}
	if len(kinds) == 0 {
		return nil, errfmt.Errorf("no valid kinds specified")
	}
	return kinds, nil
}

// FormatCLIShortcutHelpLine documents shortcut groups vs schema kinds (help demarcation).
// TRACK: BLI-1785907446155489000-2d7f745b
func FormatCLIShortcutHelpLine() string {
	return fmt.Sprintf(
		"Shortcut groups (not kind names for list/count): object splan|pplan|wstrans|evoman|draft. "+
			"Aliases: splan→%s, pplan→%s, wstrans→%s, evoman→%s.",
		kindnames.StrategicPlan, kindnames.PriorityPlan, kindnames.WorkstreamTransition, kindnames.EvolutionManagement,
	)
}
