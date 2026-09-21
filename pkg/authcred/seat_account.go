package authcred

import (
	"slices"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"gopkg.in/yaml.v3"
)

// DefaultSwarmWorkerAccount is the transitional ACC used when orchestrate cannot
// map a persona/role to a seated account.
const DefaultSwarmWorkerAccount = "ACC-1785920548450214011-dabd3692"

type accountPersonaFields struct {
	ID          string   `yaml:"id"`
	Status      string   `yaml:"status"`
	Username    string   `yaml:"username"`
	PersonaRef  string   `yaml:"persona_ref"`
	Persona     string   `yaml:"persona"`
	PersonaRefs []string `yaml:"persona_refs"`
	Roles       []string `yaml:"roles"`
	Permissions []string `yaml:"permissions"`
}

// activeAccounts is keyed by project root. Stamp is the account YAML dir.
var activeAccounts stampmemo.Table[[]accountPersonaFields]

// CanonicalAccountID resolves ACC-* passthrough or legacy account:username → ACC-*.
// Empty string means unresolved (caller keeps the original ref for diagnostics).
func CanonicalAccountID(projectRoot, ref string) string {
	raw := strings.TrimSpace(ref)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "ACC-") {
		return raw
	}
	username := raw
	if strings.HasPrefix(raw, "account:") {
		username = strings.TrimSpace(strings.TrimPrefix(raw, "account:"))
	} else if strings.Contains(raw, ":") {
		// Not an account colon-id we understand (e.g. domain:…).
		return ""
	}
	if username == "" {
		return ""
	}
	if projectRoot == "" {
		return ""
	}
	return findAccountForUsername(projectRoot, username)
}

// ResolveSeatAccount maps a persona id, role label, or ACC id to an ACC-* seat.
// Preference: already ACC-* → account with matching persona_ref → default swarm worker.
// TRACK: follow-up in kernel backlog
func ResolveSeatAccount(projectRoot, personaOrAccount string) string {
	raw := strings.TrimSpace(personaOrAccount)
	if strings.HasPrefix(raw, "ACC-") {
		return raw
	}
	if projectRoot == "" {
		return DefaultSwarmWorkerAccount
	}
	if raw != "" {
		dir := NewDiskSeatDirectory(projectRoot)
		if acc := findAccountForPersona(projectRoot, raw); acc != "" {
			return acc
		}
		if acc := findAccountForRole(projectRoot, raw); acc != "" {
			return acc
		}
		if acc := findPlannerAccount(dir, raw); acc != "" {
			return acc
		}
		if IsPlannerSeatRef(dir, raw) {
			// Planner role/account binding exists but no seated ACC — fail closed.
			return ""
		}
	}
	return DefaultSwarmWorkerAccount
}

func findPlannerAccount(dir SeatDirectory, ref string) string {
	if dir == nil {
		return ""
	}
	for _, role := range dir.Roles() {
		if !role.GrantsPlanner() || !role.Matches(ref) {
			continue
		}
		for _, acc := range dir.Accounts() {
			if role.MatchesAny(acc.Roles) {
				return acc.ID
			}
		}
	}
	return ""
}

func findAccountForPersona(projectRoot, personaID string) string {
	personaID = strings.TrimSpace(personaID)
	if personaID == "" {
		return ""
	}
	for _, acc := range loadActiveAccounts(projectRoot) {
		if strings.TrimSpace(acc.PersonaRef) == personaID || strings.TrimSpace(acc.Persona) == personaID {
			return acc.ID
		}
		for _, ref := range acc.PersonaRefs {
			if strings.TrimSpace(ref) == personaID {
				return acc.ID
			}
		}
	}
	return ""
}

func findAccountForRole(projectRoot, role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return ""
	}
	dir := NewDiskSeatDirectory(projectRoot)
	for _, acc := range dir.Accounts() {
		for _, assigned := range acc.Roles {
			if strings.EqualFold(strings.TrimSpace(assigned), role) {
				return acc.ID
			}
		}
		for _, rec := range dir.Roles() {
			if rec.Matches(role) && rec.MatchesAny(acc.Roles) {
				return acc.ID
			}
		}
	}
	return ""
}

func findAccountForUsername(projectRoot, username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}
	for _, acc := range loadActiveAccounts(projectRoot) {
		if strings.EqualFold(strings.TrimSpace(acc.Username), username) {
			return acc.ID
		}
	}
	return ""
}

// AccountCASPath returns the on-disk CAS yaml path for an ACC-* id via .account.index.
func AccountCASPath(projectRoot, accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if projectRoot == "" || !strings.HasPrefix(accountID, "ACC-") {
		return ""
	}
	hashName, ok := casHash(projectRoot, accountID, paths.AccountIndexPath(projectRoot))
	if !ok {
		return ""
	}
	return paths.AccountYAMLPath(projectRoot, hashName)
}

func loadActiveAccounts(projectRoot string) []accountPersonaFields {
	if projectRoot == "" {
		return nil
	}
	stamp := stampmemo.Of(paths.AccountIndexPath(projectRoot))
	if stamp == 0 {
		return collectActiveAccounts(projectRoot)
	}
	accs, _ := activeAccounts.Load(projectRoot, stamp, func() ([]accountPersonaFields, error) {
		return collectActiveAccounts(projectRoot), nil
	})
	return slices.Clone(accs)
}

func collectActiveAccounts(projectRoot string) []accountPersonaFields {
	indexPath := paths.AccountIndexPath(projectRoot)
	mappings := casMappings(projectRoot, indexPath)
	if mappings == nil {
		return nil
	}
	accountsDir := paths.AccountsDirPath(projectRoot)
	out := make([]accountPersonaFields, 0, len(mappings))
	for accountID := range mappings {
		if !strings.HasPrefix(accountID, "ACC-") {
			continue
		}
		raw, ok := casYAML(projectRoot, accountID, indexPath, accountsDir)
		if !ok {
			continue
		}
		var acc accountPersonaFields
		if err := yaml.Unmarshal(raw, &acc); err != nil {
			continue
		}
		if acc.ID == "" {
			acc.ID = accountID
		}
		if acc.Status != "" && acc.Status != objects.ObjectStatusActive && acc.Status != "implemented" {
			continue
		}
		out = append(out, acc)
	}
	return out
}
