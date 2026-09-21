package authcred

import (
	"slices"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"gopkg.in/yaml.v3"
)

// SeatDirectory is the external data plane for seating and planner-lane decisions.
// Behavior lives in RoleRecord / IsPlannerSeatRef; labels and permissions live here.
// TRACK: follow-up in kernel backlog
type SeatDirectory interface {
	Roles() []RoleRecord
	Accounts() []AccountRecord
}

// RoleRecord is a kernel role projection (ROL-* YAML).
type RoleRecord struct {
	ID          string
	RoleID      string
	Aliases     []string
	Permissions []string
	Status      string
}

// AccountRecord is a kernel account projection (ACC-* YAML).
type AccountRecord struct {
	ID          string
	Username    string
	Status      string
	PersonaRef  string
	Persona     string
	PersonaRefs []string
	Roles       []string
}

// MemoryDirectory is a test/fixture SeatDirectory.
type MemoryDirectory struct {
	RoleList    []RoleRecord
	AccountList []AccountRecord
}

func (m MemoryDirectory) Roles() []RoleRecord       { return m.RoleList }
func (m MemoryDirectory) Accounts() []AccountRecord { return m.AccountList }

// DiskDirectory loads roles and accounts from the project's process CAS trees.
type DiskDirectory struct {
	Root string
}

// NewDiskSeatDirectory returns a directory rooted at projectRoot (empty root is a no-op).
func NewDiskSeatDirectory(projectRoot string) SeatDirectory {
	return DiskDirectory{Root: strings.TrimSpace(projectRoot)}
}

func (d DiskDirectory) Roles() []RoleRecord {
	if d.Root == "" {
		return nil
	}
	return loadRoleRecords(d.Root)
}

func (d DiskDirectory) Accounts() []AccountRecord {
	if d.Root == "" {
		return nil
	}
	raw := loadActiveAccounts(d.Root)
	out := make([]AccountRecord, 0, len(raw))
	for _, a := range raw {
		out = append(out, AccountRecord{
			ID:          a.ID,
			Username:    a.Username,
			Status:      a.Status,
			PersonaRef:  a.PersonaRef,
			Persona:     a.Persona,
			PersonaRefs: a.PersonaRefs,
			Roles:       a.Roles,
		})
	}
	return out
}

type roleFile struct {
	ID          string   `yaml:"id"`
	Status      string   `yaml:"status"`
	RoleID      string   `yaml:"role_id"`
	Aliases     []string `yaml:"aliases"`
	Permissions []string `yaml:"permissions"`
}

// roleRecords is keyed by project root. Stamp is the role YAML dir.
var roleRecords stampmemo.Table[[]RoleRecord]

func loadRoleRecords(projectRoot string) []RoleRecord {
	if projectRoot == "" {
		return nil
	}
	stamp := stampmemo.Of(paths.RoleIndexPath(projectRoot))
	if stamp == 0 {
		return collectRoleRecords(projectRoot)
	}
	recs, _ := roleRecords.Load(projectRoot, stamp, func() ([]RoleRecord, error) {
		return collectRoleRecords(projectRoot), nil
	})
	return slices.Clone(recs)
}

func collectRoleRecords(projectRoot string) []RoleRecord {
	rolesDir := paths.RolesDirPath(projectRoot)
	indexPath := paths.RoleIndexPath(projectRoot)
	seen := map[string]struct{}{}
	var out []RoleRecord
	appendRole := func(rec RoleRecord) {
		key := rec.ID + "|" + rec.RoleID
		if key == "|" {
			return
		}
		if rec.Status != "" && rec.Status != objects.ObjectStatusActive && rec.Status != "implemented" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, rec)
	}

	indexLoaded := false
	if mappings := casMappings(projectRoot, indexPath); mappings != nil {
		indexLoaded = true
		for id := range mappings {
			raw, ok := casYAML(projectRoot, id, indexPath, rolesDir)
			if !ok {
				continue
			}
			if rec, parsed := parseRoleFile(raw); parsed {
				if rec.ID == "" {
					rec.ID = id
				}
				appendRole(rec)
			}
		}
	}

	if !indexLoaded {
		_ = forEachYAMLFile(rolesDir, func(_ string, data []byte) {
			if rec, ok := parseRoleFile(data); ok {
				appendRole(rec)
			}
		})
	}
	return out
}

func parseRoleFile(raw []byte) (RoleRecord, bool) {
	var f roleFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return RoleRecord{}, false
	}
	if f.ID == "" && f.RoleID == "" {
		return RoleRecord{}, false
	}
	return RoleRecord{
		ID:          f.ID,
		RoleID:      f.RoleID,
		Aliases:     f.Aliases,
		Permissions: f.Permissions,
		Status:      f.Status,
	}, true
}

// GrantsPlanner is true when the role object's permissions grant the planner lane.
func (r RoleRecord) GrantsPlanner() bool {
	for _, p := range r.Permissions {
		switch strings.TrimSpace(p) {
		case PermissionAgentOrchestrate, "write:*", "write:priority_plan", "write:agent_instruction", "write:strategic_plan":
			return true
		}
	}
	return false
}

// Labels is role_id + object id + aliases from the role object (no code table).
func (r RoleRecord) Labels() []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(r.RoleID)
	add(r.ID)
	add(strings.TrimPrefix(r.ID, "ROL-"))
	for _, a := range r.Aliases {
		add(a)
	}
	return out
}

// Matches reports whether assigned is this role's id, role_id, or an alias on the object.
func (r RoleRecord) Matches(assigned string) bool {
	want := strings.ToLower(strings.TrimSpace(assigned))
	if want == "" {
		return false
	}
	for _, label := range r.Labels() {
		if label == want {
			return true
		}
	}
	return false
}

// MatchesAny is Matches over a list of ACC.roles labels.
func (r RoleRecord) MatchesAny(assigned []string) bool {
	for _, a := range assigned {
		if r.Matches(a) {
			return true
		}
	}
	return false
}

func accountBindsRef(acc AccountRecord, ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	if strings.EqualFold(acc.ID, ref) || strings.EqualFold(acc.Username, ref) {
		return true
	}
	if acc.PersonaRef == ref || acc.Persona == ref {
		return true
	}
	for _, p := range acc.PersonaRefs {
		if p == ref {
			return true
		}
	}
	return false
}

func accountIsPlanner(dir SeatDirectory, acc AccountRecord) bool {
	if dir == nil {
		return false
	}
	for _, role := range dir.Roles() {
		if role.GrantsPlanner() && role.MatchesAny(acc.Roles) {
			return true
		}
	}
	return false
}

// IsPlannerSeatRef reports whether ref binds to a planner role or a planner-bound account.
// Requires a directory; an empty string or missing catalog entry is not a planner.
func IsPlannerSeatRef(dir SeatDirectory, ref string) bool {
	ref = strings.TrimSpace(ref)
	if dir == nil || ref == "" {
		return false
	}
	for _, role := range dir.Roles() {
		if role.GrantsPlanner() && role.Matches(ref) {
			return true
		}
	}
	for _, acc := range dir.Accounts() {
		if accountBindsRef(acc, ref) && accountIsPlanner(dir, acc) {
			return true
		}
	}
	return false
}

// PermissionsForAssignedRoles unions permissions from roles that match ACC.roles labels.
func PermissionsForAssignedRoles(dir SeatDirectory, assigned []string) []string {
	if dir == nil {
		return nil
	}
	var out []string
	for _, role := range dir.Roles() {
		if role.MatchesAny(assigned) {
			out = append(out, role.Permissions...)
		}
	}
	return out
}
