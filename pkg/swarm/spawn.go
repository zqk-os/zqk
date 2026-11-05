package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Supported cellular swarm archetypes.
const (
	CellArchetypeNeuron = "neuron"
	CellArchetypeMuscle = "muscle"
	CellArchetypeHeart  = "heart"
	CellArchetypeLungs  = "lungs"
)

// CellArchetypeProfile describes the biological archetype characteristics.
type CellArchetypeProfile struct {
	Type           string   `json:"type" yaml:"type"`
	FocusArea      string   `json:"focus_area" yaml:"focus_area"`
	Description    string   `json:"description" yaml:"description"`
	DefaultRole    string   `json:"default_role" yaml:"default_role"`
	DefaultPersona string   `json:"default_persona" yaml:"default_persona"`
	Capabilities   []string `json:"capabilities" yaml:"capabilities"`
}

// ArchetypeProfiles maps known archetype names to their profiles.
var ArchetypeProfiles = map[string]CellArchetypeProfile{
	CellArchetypeNeuron: {
		Type:           CellArchetypeNeuron,
		FocusArea:      "strategic",
		Description:    "strategic planning, reflection",
		DefaultRole:    "strategic_planner",
		DefaultPersona: "PER-DEFAULT-ARCHITECT",
		Capabilities:   []string{"strategic_planning", "reflection", "architecture_governance"},
	},
	CellArchetypeMuscle: {
		Type:           CellArchetypeMuscle,
		FocusArea:      "tactical",
		Description:    "tactical execution, implementation, refactoring, test verification",
		DefaultRole:    "tactical_worker",
		DefaultPersona: "PER-DEFAULT-CRAFTSMAN",
		Capabilities:   []string{"tactical_execution", "code_implementation", "refactoring", "test_verification"},
	},
	CellArchetypeHeart: {
		Type:           CellArchetypeHeart,
		FocusArea:      "operations",
		Description:    "CAP loop cadence, event dispatch, milestone latching",
		DefaultRole:    "cadence_coordinator",
		DefaultPersona: "PER-DEFAULT-TPM",
		Capabilities:   []string{"cap_loop_cadence", "event_dispatch", "milestone_latching"},
	},
	CellArchetypeLungs: {
		Type:           CellArchetypeLungs,
		FocusArea:      "intake",
		Description:    "intake, external scanning, API/repo ingest",
		DefaultRole:    "intake_scanner",
		DefaultPersona: "PER-DEFAULT-INTAKE",
		Capabilities:   []string{"intake_scanning", "external_api_ingest", "repo_discovery"},
	},
}

// SpawnOptions defines parameters for provisioning a cell swarm.
type SpawnOptions struct {
	ProjectRoot     string
	TeamConfigID    string
	PriorityPlanID  string
	DryRun          bool
	Storage         storage.ObjectStorageProvider
	SecurityContext *pkgctx.SecurityContext
}

// PersonaPod represents a single provisioned agent persona pod within a cellular swarm.
type PersonaPod struct {
	PodID           string   `json:"pod_id" yaml:"pod_id"`
	PersonaRef      string   `json:"persona_ref" yaml:"persona_ref"`
	Role            string   `json:"role" yaml:"role"`
	FocusArea       string   `json:"focus_area" yaml:"focus_area"`
	Status          string   `json:"status" yaml:"status"`
	PriorityPlanRef string   `json:"priority_plan_ref,omitempty" yaml:"priority_plan_ref,omitempty"`
	Capabilities    []string `json:"capabilities" yaml:"capabilities"`
}

// SpawnManifest records the full manifest of a dynamically provisioned cellular swarm.
type SpawnManifest struct {
	TeamConfigID         string             `json:"team_config_id" yaml:"team_config_id"`
	CellArchetype        string             `json:"cell_archetype" yaml:"cell_archetype"`
	FocusArea            string             `json:"focus_area" yaml:"focus_area"`
	ArchetypeDescription string             `json:"archetype_description" yaml:"archetype_description"`
	PriorityPlanRef      string             `json:"priority_plan_ref,omitempty" yaml:"priority_plan_ref,omitempty"`
	Pods                 []PersonaPod       `json:"pods" yaml:"pods"`
	TotalPods            int                `json:"total_pods" yaml:"total_pods"`
	CreatedAt            time.Time          `json:"created_at" yaml:"created_at"`
	PackageManifest      *pack.SwarmPackage `json:"package_manifest,omitempty" yaml:"package_manifest,omitempty"`
}

// SpawnCellSwarm provisions a cellular swarm from a team_configuration archetype.
// It inspects the cell archetype (neuron, muscle, heart, lungs), provisions required
// persona pods, optionally attaches a scoped priority plan, and reports the manifest.
func SpawnCellSwarm(ctx context.Context, opts SpawnOptions) (*SpawnManifest, error) {
	teamConfigID := strings.TrimSpace(opts.TeamConfigID)
	if teamConfigID == "" {
		return nil, errfmt.Errorf("team_configuration ID is required (e.g. --team-config TCFG-001)")
	}

	projectRoot := opts.ProjectRoot
	if projectRoot == "" {
		projectRoot = paths.ResolveProjectRoot(".")
	}
	sec := opts.SecurityContext
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}

	// 1. Read team_configuration object
	teamObj, err := resolveTeamConfigObject(ctx, opts.Storage, sec, projectRoot, teamConfigID)
	if err != nil {
		return nil, err
	}

	// 2. Inspect cell archetype
	rawCellType, _ := teamObj["cell_type"].(string)
	cellType := strings.ToLower(strings.TrimSpace(rawCellType))
	if cellType == "" {
		// Fallback check in case of nested configuration
		if nested, ok := teamObj["team_configuration"].(map[string]any); ok {
			cellType = strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", nested["cell_type"])))
		}
	}

	profile, ok := ArchetypeProfiles[cellType]
	if !ok {
		return nil, errfmt.Errorf("unsupported cell archetype %q in team_configuration %s: must be neuron, muscle, heart, or lungs", cellType, teamConfigID)
	}

	focusArea, _ := teamObj["focus_area"].(string)
	if strings.TrimSpace(focusArea) == "" {
		focusArea = profile.FocusArea
	}

	// 3. Dynamically provision persona pods from persona_allocations
	allocations := parsePersonaAllocations(teamObj)
	var pods []PersonaPod

	if len(allocations) == 0 {
		// Default archetype allocation
		podID := fmt.Sprintf("pod-%s-%s-1", cellType, sanitizeRef(profile.DefaultPersona))
		pods = append(pods, PersonaPod{
			PodID:           podID,
			PersonaRef:      profile.DefaultPersona,
			Role:            profile.DefaultRole,
			FocusArea:       focusArea,
			Status:          "provisioned",
			PriorityPlanRef: opts.PriorityPlanID,
			Capabilities:    profile.Capabilities,
		})
	} else {
		for _, alloc := range allocations {
			count := alloc.Count
			if count <= 0 {
				count = 1
			}
			role := alloc.Role
			if strings.TrimSpace(role) == "" {
				role = profile.DefaultRole
			}
			for i := 1; i <= count; i++ {
				podID := fmt.Sprintf("pod-%s-%s-%d", cellType, sanitizeRef(alloc.PersonaRef), i)
				pods = append(pods, PersonaPod{
					PodID:           podID,
					PersonaRef:      alloc.PersonaRef,
					Role:            role,
					FocusArea:       focusArea,
					Status:          "provisioned",
					PriorityPlanRef: opts.PriorityPlanID,
					Capabilities:    profile.Capabilities,
				})
			}
		}
	}

	// 4. Attach scoped priority plan if specified
	planID := strings.TrimSpace(opts.PriorityPlanID)
	if planID != "" && opts.Storage != nil && !opts.DryRun {
		planObj, planErr := opts.Storage.Read(ctx, sec, planID)
		if planErr == nil && planObj != nil {
			planObj[objects.FieldKeyTeamConfigurationRef] = teamConfigID
			_ = opts.Storage.Update(ctx, sec, planID, planObj)
		}
	}

	// 5. Build SwarmPackage representation
	var agentConfigs []pack.AgentConfig
	for _, p := range pods {
		agentConfigs = append(agentConfigs, pack.AgentConfig{
			Name: p.PodID,
			Role: p.Role,
			Skills: []string{
				profile.FocusArea,
			},
			SystemPrompt: fmt.Sprintf("Cell swarm pod executing %s archetype under %s focus area.", cellType, focusArea),
		})
	}

	pkgManifest := &pack.SwarmPackage{
		Schema:               "https://zqk.dev/schemas/swarm_package_spec.schema.json",
		Name:                 fmt.Sprintf("%s-%s-swarm", strings.ToLower(teamConfigID), cellType),
		Version:              "1.0.0",
		Description:          profile.Description,
		TeamConfigurationRef: teamConfigID,
		TeamConfiguration: &pack.TeamConfiguration{
			ID:                 teamConfigID,
			CellType:           cellType,
			FocusArea:          focusArea,
			PersonaAllocations: allocationsToPack(allocations, profile),
		},
		Agents: agentConfigs,
	}

	manifest := &SpawnManifest{
		TeamConfigID:         teamConfigID,
		CellArchetype:        cellType,
		FocusArea:            focusArea,
		ArchetypeDescription: profile.Description,
		PriorityPlanRef:      planID,
		Pods:                 pods,
		TotalPods:            len(pods),
		CreatedAt:            time.Now().UTC(),
		PackageManifest:      pkgManifest,
	}

	// 6. Persist provisioned manifest into state directory
	if !opts.DryRun && projectRoot != "" {
		persistPaths := []string{
			filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, paths.MeshStateSubdir, "spawn-manifest-latest.json"),
		}
		data, mErr := json.MarshalIndent(manifest, "", "  ")
		if mErr == nil {
			for _, p := range persistPaths {
				_ = fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755)
				_ = fileutil.WriteFile(p, data, paths.FilePerm644)
			}
		}
	}

	return manifest, nil
}

type parsedAllocation struct {
	PersonaRef string
	Role       string
	Count      int
}

func parsePersonaAllocations(teamObj map[string]any) []parsedAllocation {
	var out []parsedAllocation
	raw, ok := teamObj[objects.FieldKeyPersonaAllocations]
	if !ok {
		raw, ok = teamObj["persona_allocations"]
		if !ok {
			return nil
		}
	}

	list, ok := raw.([]any)
	if !ok {
		return nil
	}

	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		pRef, _ := m[objects.FieldKeyPersonaRef].(string)
		if pRef == "" {
			pRef, _ = m["persona_ref"].(string)
		}
		if strings.TrimSpace(pRef) == "" {
			continue
		}
		role, _ := m[objects.FieldKeyRole].(string)
		if role == "" {
			role, _ = m["role"].(string)
		}
		count := 1
		if cVal, exists := m["count"]; exists {
			switch c := cVal.(type) {
			case int:
				count = c
			case float64:
				count = int(c)
			case int64:
				count = int(c)
			}
		}
		out = append(out, parsedAllocation{
			PersonaRef: pRef,
			Role:       role,
			Count:      count,
		})
	}
	return out
}

func allocationsToPack(allocs []parsedAllocation, profile CellArchetypeProfile) []pack.PersonaAllocation {
	if len(allocs) == 0 {
		return []pack.PersonaAllocation{
			{
				PersonaRef: profile.DefaultPersona,
				Role:       profile.DefaultRole,
				Count:      1,
			},
		}
	}
	out := make([]pack.PersonaAllocation, len(allocs))
	for i, a := range allocs {
		out[i] = pack.PersonaAllocation{
			PersonaRef: a.PersonaRef,
			Role:       a.Role,
			Count:      a.Count,
		}
	}
	return out
}

func resolveTeamConfigObject(ctx context.Context, store storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, projectRoot, id string) (map[string]any, error) {
	if store != nil {
		obj, err := store.Read(ctx, sec, id)
		if err == nil && obj != nil {
			return obj, nil
		}
		// Also search by kind team_configuration
		res, listErr := store.List(ctx, sec, nil, storage.ListFilter{
			Kind: "team_configuration",
			Filters: map[string]any{
				objects.FieldKeyID: id,
			},
			Limit: 1,
		})
		if listErr == nil && res != nil && len(res.Objects) > 0 {
			return res.Objects[0], nil
		}
	}

	return nil, errfmt.Errorf("team_configuration %q not found", id)
}

func sanitizeRef(ref string) string {
	lower := strings.ToLower(ref)
	lower = strings.TrimPrefix(lower, "per-")
	lower = strings.TrimPrefix(lower, "default-")
	return strings.ReplaceAll(lower, "_", "-")
}
