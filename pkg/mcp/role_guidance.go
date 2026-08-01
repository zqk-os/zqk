package mcp

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// RoleLoader is an interface for loading role objects from storage
// Uses any to avoid import cycle with storage package
type RoleLoader interface {
	LoadRole(ctx context.Context, secCtx *pkgctx.SecurityContext, roleID string) (map[string]any, error)
	LoadRolesByIDs(ctx context.Context, secCtx *pkgctx.SecurityContext, roleIDs []string) ([]map[string]any, error)
}

// CriteriaLoader is an interface for loading criteria objects
// Uses any to avoid import cycle with storage package
type CriteriaLoader interface {
	LoadCriteria(ctx context.Context, secCtx *pkgctx.SecurityContext, criteriaID string) (map[string]any, error)
	LoadCriteriaByIDs(ctx context.Context, secCtx *pkgctx.SecurityContext, criteriaIDs []string) ([]map[string]any, error)
}

// StorageProvider is an interface to avoid import cycle
// Implementations should wrap storage.ObjectStorageProvider
// The List method should return a result with an "objects" field containing []map[string]any
type StorageProvider interface {
	List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error)
	// Create creates a new object (optional: implementations used only for List may return ErrNotSupported)
	Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
	// Read retrieves an object by ID
	Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
}

// StorageResultAdapter adapts storage.ListResult to our interface
// This allows us to work with storage results without importing storage package
type StorageResultAdapter struct {
	Objects []map[string]any
	Meta    map[string]any
}

// AdaptStorageResult adapts a storage result to our interface
// This is a helper to convert storage.ListResult to our adapter format
func AdaptStorageResult(result any) *StorageResultAdapter {
	// Try to extract objects from result
	// Result should have Objects field of type []map[string]any or []any
	resultMap, ok := result.(map[string]any)
	if ok {
		objectsAny := resultMap["objects"]
		metaAny := resultMap["meta"]

		var objects []map[string]any
		if objectsSlice, ok := objectsAny.([]map[string]any); ok {
			objects = objectsSlice
		} else if objectsSlice, ok := objectsAny.([]any); ok {
			objects = make([]map[string]any, 0, len(objectsSlice))
			for _, obj := range objectsSlice {
				if objMap, ok := obj.(map[string]any); ok {
					objects = append(objects, objMap)
				}
			}
		}

		var meta map[string]any
		if metaMap, ok := metaAny.(map[string]any); ok {
			meta = metaMap
		}

		return &StorageResultAdapter{
			Objects: objects,
			Meta:    meta,
		}
	}

	// Try reflection-based extraction if map assertion fails
	// For now, return empty result
	return &StorageResultAdapter{
		Objects: nil,
		Meta:    nil,
	}
}

// RoleResponsibility represents a responsibility linked to criteria for traceability
type RoleResponsibility struct {
	Description     string   // The responsibility description
	CriteriaRefs    []string // References to criteria objects (CRIT-####)
	RequiresAck     bool     // Whether agent must acknowledge this responsibility
	ReviewFrequency string   // How often this should be reviewed (e.g., "monthly", "quarterly")
	Category        string   // Category of responsibility (e.g., "workflow", "code_quality", "security")
}

// RolePromptTemplate represents a prompt template with role-specific customization
type RolePromptTemplate struct {
	Name        string            // Prompt name (e.g., "getting_started", "query_help")
	Content     string            // Template content with placeholders
	Variables   map[string]string // Variable substitutions for role context
	Description string            // Description of the prompt
}

// RoleGuidance represents formatted guidance for a role
type RoleGuidance struct {
	RoleID              string
	Description         string
	InfluenceLevel      string
	Permissions         []string
	Responsibilities    []RoleResponsibility
	Privileges          []string                      // What the role can do
	Restrictions        []string                      // What the role cannot do
	AccessUpgradeSteps  []string                      // Steps to get more access (for limited roles)
	AccessUpgradePrompt string                        // Prompt name for more details (e.g., "role_based_access")
	WelcomeMessage      string                        // Role-specific welcome message
	WelcomeQuickStart   string                        // Role-specific quick start instructions
	PromptTemplates     map[string]RolePromptTemplate // Standard prompt suite with role-specific customization
}

// RoleGuidanceGenerator generates role-specific guidance from role objects
type RoleGuidanceGenerator struct {
	roleLoader     RoleLoader
	criteriaLoader CriteriaLoader
	storage        StorageProvider // Interface to avoid import cycle
}

// NewRoleGuidanceGenerator creates a new role guidance generator
// storageProvider can be nil - role loading will be skipped if not available
func NewRoleGuidanceGenerator(storageProvider StorageProvider) *RoleGuidanceGenerator {
	if storageProvider == nil {
		// Return generator with no-op loaders
		return &RoleGuidanceGenerator{
			roleLoader:     &noOpRoleLoader{},
			criteriaLoader: &noOpCriteriaLoader{},
			storage:        nil,
		}
	}
	return &RoleGuidanceGenerator{
		roleLoader:     &storageRoleLoader{storage: storageProvider},
		criteriaLoader: &storageCriteriaLoader{storage: storageProvider},
		storage:        storageProvider,
	}
}

// GenerateRoleGuidance generates guidance for a role from role objects
func (g *RoleGuidanceGenerator) GenerateRoleGuidance(ctx context.Context, secCtx *pkgctx.SecurityContext, roleID string) (*RoleGuidance, error) {
	roleObj, err := g.roleLoader.LoadRole(ctx, secCtx, roleID)
	if err != nil {
		// If role object not found, return nil (caller can use fallback)
		return nil, errfmt.Newf("role object not found").Wrap(err)
	}

	guidance := &RoleGuidance{
		RoleID:          roleID,
		PromptTemplates: make(map[string]RolePromptTemplate),
	}

	// Extract basic role information
	if desc, ok := roleObj[objects.FieldKeyDescription].(string); ok {
		guidance.Description = desc
	}
	if influence, ok := roleObj[objects.FieldKeyInfluenceLevel].(string); ok {
		guidance.InfluenceLevel = influence
	}
	if perms, ok := roleObj[objects.FieldKeyPermissions].([]any); ok {
		guidance.Permissions = make([]string, 0, len(perms))
		for _, p := range perms {
			if perm, ok := p.(string); ok {
				guidance.Permissions = append(guidance.Permissions, perm)
			}
		}
	}

	// Extract responsibilities (from responsibilities field or parse from description)
	guidance.Responsibilities = g.extractResponsibilities(ctx, secCtx, roleObj)

	// Extract privileges and restrictions from permissions and description
	guidance.Privileges = g.extractPrivileges(roleObj)
	guidance.Restrictions = g.extractRestrictions(roleObj)

	// Extract access upgrade instructions
	guidance.AccessUpgradeSteps = g.extractAccessUpgradeSteps(roleObj)
	if prompt, ok := roleObj["access_upgrade_prompt"].(string); ok {
		guidance.AccessUpgradePrompt = prompt
	}

	// Extract welcome messages
	if welcomeMsg, ok := roleObj["welcome_message"].(string); ok {
		guidance.WelcomeMessage = welcomeMsg
	}
	if quickStart, ok := roleObj["welcome_quick_start"].(string); ok {
		guidance.WelcomeQuickStart = quickStart
	}

	// Extract prompt templates
	guidance.PromptTemplates = g.extractPromptTemplates(roleObj)

	return guidance, nil
}

// extractResponsibilities extracts responsibilities from role object
// Supports both structured responsibilities field and parsing from description
func (g *RoleGuidanceGenerator) extractResponsibilities(ctx context.Context, secCtx *pkgctx.SecurityContext, roleObj map[string]any) []RoleResponsibility {
	var responsibilities []RoleResponsibility

	// Try structured responsibilities field first
	if respList, ok := roleObj["responsibilities"].([]any); ok {
		for _, respAny := range respList {
			if respMap, ok := respAny.(map[string]any); ok {
				resp := RoleResponsibility{}
				if desc, ok := respMap[objects.FieldKeyDescription].(string); ok {
					resp.Description = desc
				}
				if critRefs, ok := respMap[objects.FieldKeyCriteriaRefs].([]any); ok {
					resp.CriteriaRefs = make([]string, 0, len(critRefs))
					for _, ref := range critRefs {
						if refStr, ok := ref.(string); ok {
							resp.CriteriaRefs = append(resp.CriteriaRefs, refStr)
						}
					}
				}
				if requiresAck, ok := respMap["requires_ack"].(bool); ok {
					resp.RequiresAck = requiresAck
				}
				if reviewFreq, ok := respMap["review_frequency"].(string); ok {
					resp.ReviewFrequency = reviewFreq
				}
				if category, ok := respMap[objects.FieldKeyCategory].(string); ok {
					resp.Category = category
				}
				responsibilities = append(responsibilities, resp)
			}
		}
		return responsibilities
	}

	// Fallback: Parse from description if structured field not available
	// This maintains backward compatibility
	if desc, ok := roleObj[objects.FieldKeyDescription].(string); ok && desc != emptyValue {
		// For now, create a single responsibility from description
		// In the future, role objects should have structured responsibilities field
		responsibilities = append(responsibilities, RoleResponsibility{
			Description:     desc,
			CriteriaRefs:    []string{}, // No criteria refs in fallback
			RequiresAck:     false,
			ReviewFrequency: emptyValue,
			Category:        "general",
		})
	}

	return responsibilities
}

// extractPrivileges extracts what the role can do from permissions and description
func (g *RoleGuidanceGenerator) extractPrivileges(roleObj map[string]any) []string {
	var privileges []string

	// Extract from permissions
	if perms, ok := roleObj[objects.FieldKeyPermissions].([]any); ok {
		for _, p := range perms {
			if perm, ok := p.(string); ok {
				// Format permission as privilege
				if strings.HasSuffix(perm, ":*") {
					op := strings.TrimSuffix(perm, ":*")
					privileges = append(privileges, fmt.Sprintf("Can %s all objects", op))
				} else {
					privileges = append(privileges, fmt.Sprintf("Has permission: %s", perm))
				}
			}
		}
	}

	return privileges
}

// extractRestrictions extracts what the role cannot do
func (g *RoleGuidanceGenerator) extractRestrictions(roleObj map[string]any) []string {
	var restrictions []string

	// Extract from permissions - infer restrictions
	if perms, ok := roleObj[objects.FieldKeyPermissions].([]any); ok {
		hasWriteAll := false
		hasDeleteAll := false
		for _, p := range perms {
			if perm, ok := p.(string); ok {
				if perm == "write:*" {
					hasWriteAll = true
				}
				if perm == "delete:*" {
					hasDeleteAll = true
				}
			}
		}

		if !hasWriteAll {
			restrictions = append(restrictions, "Cannot write all object types (limited write access)")
		}
		if !hasDeleteAll {
			restrictions = append(restrictions, "Cannot delete objects (read-only or limited delete access)")
		}
	}

	return restrictions
}

// extractAccessUpgradeSteps extracts access upgrade instructions from role object
func (g *RoleGuidanceGenerator) extractAccessUpgradeSteps(roleObj map[string]any) []string {
	var steps []string

	// Try structured access_upgrade_steps field
	if stepsList, ok := roleObj["access_upgrade_steps"].([]any); ok {
		for _, stepAny := range stepsList {
			if step, ok := stepAny.(string); ok {
				steps = append(steps, step)
			}
		}
		return steps
	}

	// Fallback: Try access_upgrade_instructions field (alternative name)
	if instructions, ok := roleObj["access_upgrade_instructions"].(string); ok && instructions != emptyValue {
		// Split by newlines if it's a multi-line string
		for line := range strings.SplitSeq(instructions, "\n") {
			line = strings.TrimSpace(line)
			if line != emptyValue {
				steps = append(steps, line)
			}
		}
		return steps
	}

	return steps
}

// extractPromptTemplates extracts prompt templates from role object
func (g *RoleGuidanceGenerator) extractPromptTemplates(roleObj map[string]any) map[string]RolePromptTemplate {
	templates := make(map[string]RolePromptTemplate)

	// Try structured prompt_templates field
	if templatesList, ok := roleObj["prompt_templates"].([]any); ok {
		for _, templateAny := range templatesList {
			if templateMap, ok := templateAny.(map[string]any); ok {
				template := RolePromptTemplate{}
				if name, ok := templateMap[objects.FieldKeyName].(string); ok {
					template.Name = name
				}
				if content, ok := templateMap[objects.FieldKeyContent].(string); ok {
					template.Content = content
				}
				if desc, ok := templateMap[objects.FieldKeyDescription].(string); ok {
					template.Description = desc
				}
				if vars, ok := templateMap["variables"].(map[string]any); ok {
					template.Variables = make(map[string]string)
					for k, v := range vars {
						if vStr, ok := v.(string); ok {
							template.Variables[k] = vStr
						}
					}
				}
				if template.Name != emptyValue {
					templates[template.Name] = template
				}
			}
		}
		return templates
	}

	// Fallback: Try individual prompt fields (e.g., "prompt_getting_started", "prompt_query_help")
	// This allows backward compatibility with simpler role object structures
	promptFields := []string{
		"getting_started", "query_help", "common_tasks", "object_lifecycle",
		"filter_syntax", "role_based_access", "create_object_template",
	}
	for _, promptName := range promptFields {
		fieldName := fmt.Sprintf("prompt_%s", promptName)
		if content, ok := roleObj[fieldName].(string); ok && content != emptyValue {
			templates[promptName] = RolePromptTemplate{
				Name:    promptName,
				Content: content,
			}
		}
	}

	return templates
}

// FormatRoleGuidance formats role guidance as markdown
func (g *RoleGuidanceGenerator) FormatRoleGuidance(ctx context.Context, secCtx *pkgctx.SecurityContext, guidance *RoleGuidance) string {
	if guidance == nil {
		return emptyValue
	}

	var parts []string

	parts = append(parts, fmt.Sprintf("## Your Role: %s\n\n", guidance.RoleID))

	if guidance.Description != emptyValue {
		parts = append(parts, fmt.Sprintf("**Description**: %s\n\n", guidance.Description))
	}

	if guidance.InfluenceLevel != emptyValue {
		parts = append(parts, fmt.Sprintf("**Influence Level**: %s\n\n", guidance.InfluenceLevel))
	}

	// Responsibilities
	if len(guidance.Responsibilities) > 0 {
		parts = append(parts, "### Your Responsibilities\n\n")
		for _, resp := range guidance.Responsibilities {
			parts = append(parts, fmt.Sprintf("- **%s**: %s", resp.Category, resp.Description))

			// Add criteria references for traceability
			if len(resp.CriteriaRefs) > 0 {
				parts = append(parts, fmt.Sprintf("  - *Traceable to criteria: %s*", strings.Join(resp.CriteriaRefs, ", ")))
			}

			// Add acknowledgment requirement
			if resp.RequiresAck {
				parts = append(parts, "  - *⚠️ Requires acknowledgment*")
			}

			// Add review frequency
			if resp.ReviewFrequency != emptyValue {
				parts = append(parts, fmt.Sprintf("  - *Review frequency: %s*", resp.ReviewFrequency))
			}

			parts = append(parts, "\n")
		}
		parts = append(parts, "\n")
	}

	// Privileges
	if len(guidance.Privileges) > 0 {
		parts = append(parts, "### What You Can Do\n\n")
		for _, priv := range guidance.Privileges {
			parts = append(parts, fmt.Sprintf("- ✅ %s\n", priv))
		}
		parts = append(parts, "\n")
	}

	// Restrictions
	if len(guidance.Restrictions) > 0 {
		parts = append(parts, "### Restrictions\n\n")
		for _, restr := range guidance.Restrictions {
			parts = append(parts, fmt.Sprintf("- ❌ %s\n", restr))
		}
		parts = append(parts, "\n")
	}

	return strings.Join(parts, "")
}

// storageRoleLoader implements RoleLoader using storage provider
type storageRoleLoader struct {
	storage StorageProvider
}

func (l *storageRoleLoader) LoadRole(ctx context.Context, secCtx *pkgctx.SecurityContext, roleID string) (map[string]any, error) {
	if l.storage == nil {
		return nil, errfmt.Errorf("storage provider not available")
	}

	storageCtx := pkgctx.NewStorageContext()

	// Query by role_id field
	// Filter format matches storage.ListFilter structure
	// Using map[string]any to avoid importing storage.ListFilter (import cycle)
	filter := map[string]any{
		objects.FieldKeyKind: objects.KindRole,
		"filters": map[string]any{
			objects.FieldKeyRoleID: roleID,
		},
		"limit": 1,
	}

	resultAny, err := l.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to query role").Wrap(err)
	}

	// Adapt result to our interface
	adapter := AdaptStorageResult(resultAny)
	if len(adapter.Objects) == 0 {
		return nil, errfmt.Errorf("role not found: %s", roleID)
	}

	return adapter.Objects[0], nil
}

func (l *storageRoleLoader) LoadRolesByIDs(ctx context.Context, secCtx *pkgctx.SecurityContext, roleIDs []string) ([]map[string]any, error) {
	var roles []map[string]any
	for _, roleID := range roleIDs {
		role, err := l.LoadRole(ctx, secCtx, roleID)
		if err != nil {
			// Log error but continue with other roles
			continue
		}
		roles = append(roles, role)
	}
	return roles, nil
}

// storageCriteriaLoader implements CriteriaLoader using storage provider
type storageCriteriaLoader struct {
	storage StorageProvider
}

func (l *storageCriteriaLoader) LoadCriteria(ctx context.Context, secCtx *pkgctx.SecurityContext, criteriaID string) (map[string]any, error) {
	if l.storage == nil {
		return nil, errfmt.Errorf("storage provider not available")
	}

	storageCtx := pkgctx.NewStorageContext()

	filter := map[string]any{
		objects.FieldKeyKind: objects.KindCriteria,
		"filters": map[string]any{
			objects.FieldKeyID: criteriaID,
		},
		"limit": 1,
	}

	resultAny, err := l.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to query criteria").Wrap(err)
	}

	// Adapt result to our interface
	adapter := AdaptStorageResult(resultAny)
	if len(adapter.Objects) == 0 {
		return nil, errfmt.Errorf("criteria not found: %s", criteriaID)
	}

	return adapter.Objects[0], nil
}

func (l *storageCriteriaLoader) LoadCriteriaByIDs(ctx context.Context, secCtx *pkgctx.SecurityContext, criteriaIDs []string) ([]map[string]any, error) {
	var criteria []map[string]any
	for _, criteriaID := range criteriaIDs {
		crit, err := l.LoadCriteria(ctx, secCtx, criteriaID)
		if err != nil {
			// Log error but continue with other criteria
			continue
		}
		criteria = append(criteria, crit)
	}
	return criteria, nil
}

// noOpRoleLoader is a no-op implementation when storage is not available
type noOpRoleLoader struct{}

func (l *noOpRoleLoader) LoadRole(ctx context.Context, secCtx *pkgctx.SecurityContext, roleID string) (map[string]any, error) {
	return nil, errfmt.Errorf("storage not available")
}

func (l *noOpRoleLoader) LoadRolesByIDs(ctx context.Context, secCtx *pkgctx.SecurityContext, roleIDs []string) ([]map[string]any, error) {
	return nil, errfmt.Errorf("storage not available")
}

// noOpCriteriaLoader is a no-op implementation when storage is not available
type noOpCriteriaLoader struct{}

func (l *noOpCriteriaLoader) LoadCriteria(ctx context.Context, secCtx *pkgctx.SecurityContext, criteriaID string) (map[string]any, error) {
	return nil, errfmt.Errorf("storage not available")
}

func (l *noOpCriteriaLoader) LoadCriteriaByIDs(ctx context.Context, secCtx *pkgctx.SecurityContext, criteriaIDs []string) ([]map[string]any, error) {
	return nil, errfmt.Errorf("storage not available")
}
