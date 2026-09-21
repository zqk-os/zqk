package authcred

import (
	"slices"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

type boundParseHit struct {
	raw []byte
	acc BoundAccount
}

var boundParsed sync.Map // projectRoot + "\x00" + accountID -> boundParseHit

// BoundAccount is the RBAC slice of an ACC-* object needed at auth time.
type BoundAccount struct {
	ID          string
	Roles       []string
	PersonaRef  string
	Persona     string
	PersonaRefs []string
	Permissions []string
}

// LookupBoundAccount loads one ACC-* via the cached listing index.
func LookupBoundAccount(projectRoot, accountID string) (BoundAccount, error) {
	accountID = strings.TrimSpace(accountID)
	if projectRoot == "" || accountID == "" {
		return BoundAccount{}, errfmt.Errorf("unauthorized: account index missing; cannot verify role+persona binding for %s (CRI-ACCOUNT-RBAC-READY)", accountID)
	}
	indexPath := paths.AccountIndexPath(projectRoot)
	mappings := casMappings(projectRoot, indexPath)
	if mappings == nil {
		return BoundAccount{}, errfmt.Errorf("unauthorized: account index missing; cannot verify role+persona binding for %s (CRI-ACCOUNT-RBAC-READY)", accountID)
	}
	if _, ok := mappings[accountID]; !ok {
		return BoundAccount{}, errfmt.Errorf("unauthorized: account %s not found in account index (POL-AGENT-ACCOUNT-LOGIN-001)", accountID)
	}
	raw, ok := casYAML(projectRoot, accountID, indexPath, paths.AccountsDirPath(projectRoot))
	if !ok {
		return BoundAccount{}, errfmt.Errorf("unauthorized: failed to read account %s", accountID)
	}
	cacheKey := projectRoot + "\x00" + accountID
	if hit, ok := boundParsed.Load(cacheKey); ok {
		parsed := hit.(boundParseHit)
		if sameByteBacking(parsed.raw, raw) {
			return cloneBoundAccount(parsed.acc), nil
		}
	}
	var acc accountPersonaFields
	if err := yaml.Unmarshal(raw, &acc); err != nil {
		return BoundAccount{}, errfmt.Errorf("unauthorized: failed to parse account %s: %w", accountID, err)
	}
	if acc.ID == "" {
		acc.ID = accountID
	}
	bound := BoundAccount{
		ID:          acc.ID,
		Roles:       acc.Roles,
		PersonaRef:  acc.PersonaRef,
		Persona:     acc.Persona,
		PersonaRefs: acc.PersonaRefs,
		Permissions: acc.Permissions,
	}
	boundParsed.Store(cacheKey, boundParseHit{raw: raw, acc: bound})
	return cloneBoundAccount(bound), nil
}

func cloneBoundAccount(acc BoundAccount) BoundAccount {
	acc.Roles = slices.Clone(acc.Roles)
	acc.PersonaRefs = slices.Clone(acc.PersonaRefs)
	acc.Permissions = slices.Clone(acc.Permissions)
	return acc
}

func sameByteBacking(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
}
