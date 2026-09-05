package authcred

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// DefaultSwarmWorkerAccount is the transitional ACC used when orchestrate cannot
// map a persona/role to a seated account.
// TRACK: REDACTED — replace with seating registry object.
const DefaultSwarmWorkerAccount = "ACC-1785920548450214011-dabd3692"

// legacyAccountColonToACC mirrors scripts/acc-migrate-map.json for offline
// canonicalize of retired account:username ids (POL-AGENT-ACCOUNT-LOGIN-001).
// TRACK: REDACTED — drop when no account: refs remain in CAS.
var legacyAccountColonToACC = map[string]string{
	"account:coder_agent":             "ACC-1785920548450214000-80bb9c63",
	"account:ide-seat-01":             "ACC-1785920548450214001-7b3cc2de",
	"account:default":                 "ACC-1785920548450214002-fe08aa1e",
	"account:developer":               "ACC-1785920548450214003-23d25bd5",
	"account:executive":               "ACC-1785920548450214004-ad421786",
	"account:founder":                 "ACC-1785920548450214005-60837d47",
	"account:lanceettl":               "ACC-1785920548450214006-2e52b3e7",
	"account:observer_agent":          "ACC-1785920548450214007-10c6d625",
	"account:owner":                   "ACC-1785920548450214008-ce03e2b5",
	"account:pedantic-code-inspector": "ACC-1785920548450214009-c051e765",
	"account:senior_dev":              "ACC-1785920548450214010-8695c409",
	"account:swarm_worker":            DefaultSwarmWorkerAccount,
	"account:system":                  "ACC-1785920548450214012-68b850c0",
	"account:system-auditor":          "ACC-1785920548450214013-32ac8ee9",
	"account:team_alpha":              "ACC-1785920548450214014-426d9b87",
	"account:test-user":               "ACC-1785920548450214015-3df55bd1",
	"account:test_agent":              "ACC-1785920548450214016-ace2aae1",
	"account:viewer":                  "ACC-1785920548450214017-87f10a62",
}

type accountIndexFile struct {
	Mappings map[string]string `json:"mappings"`
}

type accountPersonaFields struct {
	ID          string   `yaml:"id"`
	Status      string   `yaml:"status"`
	Username    string   `yaml:"username"`
	PersonaRef  string   `yaml:"persona_ref"`
	Persona     string   `yaml:"persona"`
	PersonaRefs []string `yaml:"persona_refs"`
	Roles       []string `yaml:"roles"`
}

// CanonicalAccountID resolves ACC-* passthrough or legacy account:username → ACC-*.
// Empty string means unresolved (caller keeps the original ref for diagnostics).
// TRACK: REDACTED
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
	legacyKey := "account:" + strings.ToLower(username)
	if acc, ok := legacyAccountColonToACC[legacyKey]; ok {
		return acc
	}
	if acc, ok := legacyAccountColonToACC["account:"+username]; ok {
		return acc
	}
	if projectRoot == "" {
		return ""
	}
	return findAccountForUsername(projectRoot, username)
}

// ResolveSeatAccount maps a persona id, role label, or ACC id to an ACC-* seat.
// Preference: already ACC-* → account with matching persona_ref → default swarm worker.
// TRACK: REDACTED
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
// TRACK: REDACTED
func AccountCASPath(projectRoot, accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if projectRoot == "" || !strings.HasPrefix(accountID, "ACC-") {
		return ""
	}
	indexPath := filepath.Join(projectRoot, paths.ProcessDir, "accounts", ".account.index")
	data, err := fileutil.ReadFile(indexPath)
	if err != nil {
		return ""
	}
	var idx accountIndexFile
	if err := json.Unmarshal(data, &idx); err != nil || idx.Mappings == nil {
		return ""
	}
	hashName, ok := idx.Mappings[accountID]
	if !ok || strings.TrimSpace(hashName) == "" {
		return ""
	}
	return filepath.Join(projectRoot, paths.ProcessDir, "accounts", hashName+".yaml")
}

func loadActiveAccounts(projectRoot string) []accountPersonaFields {
	indexPath := filepath.Join(projectRoot, paths.ProcessDir, "accounts", ".account.index")
	data, err := fileutil.ReadFile(indexPath)
	if err != nil {
		return nil
	}
	var idx accountIndexFile
	if err := json.Unmarshal(data, &idx); err != nil || idx.Mappings == nil {
		return nil
	}
	out := make([]accountPersonaFields, 0, len(idx.Mappings))
	for accountID, hashName := range idx.Mappings {
		if !strings.HasPrefix(accountID, "ACC-") {
			continue
		}
		yamlPath := filepath.Join(projectRoot, paths.ProcessDir, "accounts", hashName+".yaml")
		raw, err := fileutil.ReadFile(yamlPath)
		if err != nil {
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
