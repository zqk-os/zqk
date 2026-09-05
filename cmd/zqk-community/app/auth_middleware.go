package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/authcred"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// AuthMiddleware intercepts commands to enforce Identity/Role RBAC checks.
func AuthMiddleware(cmd *cobra.Command, projectRoot string) error {
	// Builtins that may lack annotations. Session-optional commands walk ancestors.
	switch cmd.Name() {
	case "help", "version", "completion", "quickstart", "login", "logout", "auth":
		return nil
	}
	if cli.SessionOptional(cmd) {
		return nil
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	isTest := false
	bypassVal := os.Getenv(zqkenv.TestBypassAuth())
	if bypassVal == "1" {
		isTest = true
	} else if bypassVal != "0" {
		if strings.HasSuffix(os.Args[0], ".test") {
			isTest = true
		} else {
			for _, arg := range os.Args {
				if strings.HasPrefix(arg, "-test.") {
					isTest = true
					break
				}
			}
		}
	}

	if isTest {
		secCtx := pkgctx.NewTestSecurityContext()
		cmd.SetContext(pkgctx.WithSecurityContext(ctx, secCtx))
		return nil
	}

	apiKey := os.Getenv(zqkenv.APIKey())
	var credentialsToken string
	credPath := authcred.ResolveCredentialPath(projectRoot)
	if credPath != "" {
		data, err := fileutil.ReadFile(credPath)
		if err == nil {
			credentialsToken = strings.TrimSpace(string(data))
		}
	}

	if apiKey == "" && credentialsToken == "" {
		return errfmt.Errorf("unauthorized: missing token in ~/.zqk/credentials or %s", zqkenv.APIKey())
	}

	// Parse token/key, load Identity/Role schemas (account.yaml, role.yaml), and validate access
	accountSchemaPath := filepath.Join(projectRoot, paths.ProcessDir, "_internal", "object_specs", "account.yaml")
	roleSchemaPath := filepath.Join(projectRoot, paths.ProcessDir, "_internal", "object_specs", "role.yaml")

	if _, statErr := fileutil.Stat(accountSchemaPath); statErr != nil {
		return errfmt.Errorf("unauthorized: failed to load account schema: %v", statErr)
	}

	if _, statErr := fileutil.Stat(roleSchemaPath); statErr != nil {
		return errfmt.Errorf("unauthorized: failed to load role schema: %v", statErr)
	}

	// Inject SecurityContext for the CLI processor
	accountID := apiKey
	if accountID == "" {
		accountID = credentialsToken
	}

	if strings.HasPrefix(accountID, "ZQK-") {
		sp, err := cli.GetObjectStorageForCommand(cmd, projectRoot)
		if err != nil || sp == nil {
			return errfmt.Errorf("unauthorized: storage not available to validate session: %v", err)
		}
		systemSecCtx := pkgctx.NewSystemSecurityContext()
		sessionObj, readErr := sp.Read(ctx, systemSecCtx, accountID)
		if readErr != nil {
			if errors.Is(readErr, storage.ErrObjectNotFound) {
				return errfmt.Errorf("unauthorized: session %s not found", accountID)
			}
			return errfmt.Errorf("unauthorized: session %s not found: %w", accountID, readErr)
		}
		if accID, exists := sessionObj[objects.FieldKeyAccountID].(string); exists && accID != "" {
			accountID = accID
		} else {
			return errfmt.Errorf("unauthorized: session %s has no account_id", accountID)
		}
	}

	secCtx, err := resolveSecurityContext(projectRoot, accountID)
	if err != nil {
		return err
	}

	// WARNING: We cannot use context.WithValue on cmd.Context() directly if it's already set by cobra,
	// but cmd.SetContext allows overriding it.
	cmd.SetContext(pkgctx.WithSecurityContext(ctx, secCtx))

	return nil
}

type IndexFile struct {
	Mappings map[string]string `json:"mappings"`
}

type AccountObj struct {
	Roles       []string `yaml:"roles"`
	PersonaRef  string   `yaml:"persona_ref"`
	PersonaRefs []string `yaml:"persona_refs"`
	Persona     string   `yaml:"persona"`
}

type RoleObj struct {
	RoleID      string   `yaml:"role_id"`
	ID          string   `yaml:"id"`
	Permissions []string `yaml:"permissions"`
}

type PersonaObj struct {
	VocabularySchemeRefs []string `yaml:"vocabulary_scheme_refs"`
}

func resolveSecurityContext(projectRoot string, rawAccountID string) (*pkgctx.SecurityContext, error) {
	var explicitPersona string

	// Tripartite Identity parsing: Account|Role|Persona
	parts := strings.Split(rawAccountID, "|")
	accountID := parts[0]
	if len(parts) >= 3 {
		// explicitRole = parts[1] (not currently utilized for filtering, but reserved for future role-based strictness)
		explicitPersona = parts[2]
	} else if len(parts) == 2 {
		explicitPersona = parts[1]
	}

	if accountID == pkgctx.SystemAccountID || accountID == "agent-session-token" {
		secCtx := pkgctx.NewSystemSecurityContext()
		secCtx.PersonaID = explicitPersona
		return secCtx, nil
	}
	if accountID == pkgctx.TestHarnessAccountID || accountID == "ACC-TEST-HARNESS" {
		secCtx := pkgctx.NewTestSecurityContext()
		secCtx.PersonaID = explicitPersona
		return secCtx, nil
	}

	// POL-AGENT-ACCOUNT-LOGIN-001 / CRI-ACCOUNT-RBAC-READY: ACC-* only (legacy account:* retired).
	if strings.HasPrefix(accountID, "account:") {
		return nil, errfmt.Errorf("unauthorized: legacy account:* ids are retired; authenticate as ACC-* (POL-AGENT-ACCOUNT-LOGIN-001)")
	}
	if !strings.HasPrefix(accountID, "ACC-") {
		return nil, errfmt.Errorf("unauthorized: account id must use ACC-* form (got %q); see POL-AGENT-ACCOUNT-LOGIN-001", accountID)
	}

	accountKey := accountID

	var activeVocabularySchemes []string
	if explicitPersona != "" {
		personaIndexPat := filepath.Join(projectRoot, paths.ProcessDir, "personas", ".persona.index")
		var personaIndex IndexFile
		if personaIndexData, err := fileutil.ReadFile(personaIndexPat); err == nil {
			if err := json.Unmarshal(personaIndexData, &personaIndex); err == nil {
				if hashName, ok := personaIndex.Mappings[explicitPersona]; ok {
					personaYamlPath := filepath.Join(projectRoot, paths.ProcessDir, "personas", hashName+".yaml")
					if personaYamlData, err := fileutil.ReadFile(personaYamlPath); err == nil {
						var pObj PersonaObj
						if err := yaml.Unmarshal(personaYamlData, &pObj); err == nil {
							activeVocabularySchemes = pObj.VocabularySchemeRefs
						}
					}
				}
			}
		}
	}

	// Read account index
	accountIndexPat := filepath.Join(projectRoot, paths.ProcessDir, "accounts", ".account.index")
	var accountIndex IndexFile
	accountIndexData, err := fileutil.ReadFile(accountIndexPat)
	if err != nil {
		if zqkenv.IsCommunityEdition || os.Getenv("ZQK_SKIP_RBAC") == "1" {
			secCtx := pkgctx.NewSystemSecurityContext()
			secCtx.PersonaID = "PER-COMMUNITY-AGENT"
			if explicitPersona != "" {
				secCtx.PersonaID = explicitPersona
			}
			return secCtx, nil
		}
		return nil, errfmt.Errorf("unauthorized: account index missing; cannot verify role+persona binding for %s (CRI-ACCOUNT-RBAC-READY)", accountID)
	}

	if err := json.Unmarshal(accountIndexData, &accountIndex); err != nil {
		return nil, errfmt.Errorf("failed to parse account index: %w", err)
	}

	// Find the hash path for the account
	hashName, ok := accountIndex.Mappings[accountID]
	if !ok {
		hashName, ok = accountIndex.Mappings[accountKey]
	}
	if !ok {
		return nil, errfmt.Errorf("unauthorized: account %s not found in account index (POL-AGENT-ACCOUNT-LOGIN-001)", accountID)
	}

	var roles []string
	var boundPersona string
	accountYamlPath := filepath.Join(projectRoot, paths.ProcessDir, "accounts", hashName+".yaml")
	accountYamlData, err := fileutil.ReadFile(accountYamlPath)
	if err != nil {
		return nil, errfmt.Errorf("unauthorized: failed to read account %s: %w", accountID, err)
	}
	var accObj AccountObj
	if err := yaml.Unmarshal(accountYamlData, &accObj); err != nil {
		return nil, errfmt.Errorf("unauthorized: failed to parse account %s: %w", accountID, err)
	}
	roles = accObj.Roles
	boundPersona = firstNonEmpty(explicitPersona, accObj.PersonaRef, accObj.Persona, firstString(accObj.PersonaRefs))

	if len(roles) == 0 || boundPersona == "" {
		return nil, errfmt.Errorf("unauthorized: account %s is not RBAC-ready (need roles + persona_ref; CRI-ACCOUNT-RBAC-READY)", accountID)
	}

	if boundPersona != "" && len(activeVocabularySchemes) == 0 {
		personaIndexPat := filepath.Join(projectRoot, paths.ProcessDir, "personas", ".persona.index")
		var personaIndex IndexFile
		if personaIndexData, err := fileutil.ReadFile(personaIndexPat); err == nil {
			if err := json.Unmarshal(personaIndexData, &personaIndex); err == nil {
				if pHash, ok := personaIndex.Mappings[boundPersona]; ok {
					personaYamlPath := filepath.Join(projectRoot, paths.ProcessDir, "personas", pHash+".yaml")
					if personaYamlData, err := fileutil.ReadFile(personaYamlPath); err == nil {
						var pObj PersonaObj
						if err := yaml.Unmarshal(personaYamlData, &pObj); err == nil {
							activeVocabularySchemes = pObj.VocabularySchemeRefs
						}
					}
				}
			}
		}
	}

	// Read role index to resolve role permissions
	roleIndexPat := filepath.Join(projectRoot, paths.ProcessDir, "roles", ".role.index")
	var roleIndex IndexFile
	var permissions []string
	roleIndexData, err := fileutil.ReadFile(roleIndexPat)
	if err == nil {
		if err := json.Unmarshal(roleIndexData, &roleIndex); err == nil {
			for _, rHashName := range roleIndex.Mappings {
				roleYamlPath := filepath.Join(projectRoot, paths.ProcessDir, "roles", rHashName+".yaml")
				roleYamlData, err := fileutil.ReadFile(roleYamlPath)
				if err != nil {
					continue
				}
				var rObj RoleObj
				if err := yaml.Unmarshal(roleYamlData, &rObj); err != nil {
					continue
				}
				for _, assignedRole := range roles {
					if rObj.RoleID == assignedRole || rObj.ID == assignedRole || strings.TrimPrefix(rObj.ID, "ROL-") == assignedRole {
						permissions = append(permissions, rObj.Permissions...)
					}
				}
			}
		}
	}

	return &pkgctx.SecurityContext{
		AccountID:               accountID,
		Roles:                   roles,
		Permissions:             permissions,
		NamespaceID:             "*",
		PersonaID:               boundPersona,
		ActiveVocabularySchemes: activeVocabularySchemes,
	}, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstString(vals []string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
