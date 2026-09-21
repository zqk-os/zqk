package authcred

import (
	"slices"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"gopkg.in/yaml.v3"
)

var boundAccounts stampmemo.View[BoundAccount]

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
	bound, err := boundAccounts.Get(projectRoot+"\x00"+accountID, raw, func(raw []byte) (BoundAccount, error) {
		var acc accountPersonaFields
		if err := yaml.Unmarshal(raw, &acc); err != nil {
			return BoundAccount{}, errfmt.Errorf("unauthorized: failed to parse account %s: %w", accountID, err)
		}
		if acc.ID == "" {
			acc.ID = accountID
		}
		return BoundAccount{
			ID:          acc.ID,
			Roles:       acc.Roles,
			PersonaRef:  acc.PersonaRef,
			Persona:     acc.Persona,
			PersonaRefs: acc.PersonaRefs,
			Permissions: acc.Permissions,
		}, nil
	})
	if err != nil {
		return BoundAccount{}, err
	}
	return cloneBoundAccount(bound), nil
}

func cloneBoundAccount(acc BoundAccount) BoundAccount {
	acc.Roles = slices.Clone(acc.Roles)
	acc.PersonaRefs = slices.Clone(acc.PersonaRefs)
	acc.Permissions = slices.Clone(acc.Permissions)
	return acc
}
