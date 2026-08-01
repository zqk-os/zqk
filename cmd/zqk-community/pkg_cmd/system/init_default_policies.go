package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"gopkg.in/yaml.v3"
)

const defaultPoliciesRelDir = "scripts/default_policies"

// SeedDefaultPolicyPack creates curated default policy objects after greenfield init
// (ITEM-REDACTED-19677269 / DEC-REDACTED-7b756414).
// Sources YAML from scripts/default_policies (project or source repo), falling back to
// an embedded minimal pack when the directory is absent.
// Idempotent: skips when a policy with the same stable title already exists.
func SeedDefaultPolicyPack(projectRoot string, logger logging.Logger) (created int, err error) {
	if projectRoot == emptyValue {
		return 0, errfmt.Errorf("project root is empty")
	}
	ctx := pkgctx.NewSystemContext()
	factory, ferr := storage.NewStorageFactory(ctx, projectRoot)
	if ferr != nil {
		return 0, errfmt.Newf("storage factory for default policy pack").Wrap(ferr)
	}
	sp := factory.GetStorage()
	if sp == nil {
		return 0, errfmt.Errorf("storage provider is nil")
	}
	secCtx := pkgctx.NewSystemSecurityContext()

	existingTitles, lerr := listExistingPolicyTitles(ctx, sp, secCtx)
	if lerr != nil {
		logging.Fluent(logger).Warn("Could not list existing policies before seeding; will attempt creates").
			WithError(lerr).
			Log()
		existingTitles = map[string]bool{}
	}

	templates, terr := loadDefaultPolicyTemplates(projectRoot)
	if terr != nil {
		return 0, terr
	}
	if len(templates) == 0 {
		return 0, errfmt.Errorf("no default policy templates found under %s", defaultPoliciesRelDir)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, tmpl := range templates {
		title, _ := tmpl[objects.FieldKeyTitle].(string)
		if title == emptyValue {
			continue
		}
		if existingTitles[strings.ToLower(title)] {
			continue
		}
		obj := map[string]any{}
		for k, v := range tmpl {
			obj[k] = v
		}
		obj[objects.FieldKeyKind] = objects.KindPolicy
		if objects.GetString(obj, objects.FieldKeySchemaVersion) == emptyValue {
			obj[objects.FieldKeySchemaVersion] = objects.DefaultSchemaVersion
		}
		if objects.GetString(obj, objects.FieldKeyStatus) == emptyValue {
			obj[objects.FieldKeyStatus] = objects.ObjectStatusActive
		}
		if objects.GetString(obj, objects.FieldKeyID) == emptyValue {
			obj[objects.FieldKeyID] = stableDefaultPolicyID(title)
		}
		if objects.GetString(obj, objects.FieldKeyCreatedAt) == emptyValue {
			obj[objects.FieldKeyCreatedAt] = now
		}
		if objects.GetString(obj, objects.FieldKeyUpdatedAt) == emptyValue {
			obj[objects.FieldKeyUpdatedAt] = now
		}
		if objects.GetString(obj, objects.FieldKeyCreatedBy) == emptyValue {
			obj[objects.FieldKeyCreatedBy] = pkgctx.SystemAccountID
		}
		if objects.GetString(obj, objects.FieldKeyUpdatedBy) == emptyValue {
			obj[objects.FieldKeyUpdatedBy] = pkgctx.SystemAccountID
		}
		if cerr := sp.Create(ctx, secCtx, obj); cerr != nil {
			errLower := strings.ToLower(cerr.Error())
			if strings.Contains(errLower, "already exists") || strings.Contains(errLower, "duplicate") {
				continue
			}
			return created, errfmt.Newf("create default policy %q", title).Wrap(cerr)
		}
		created++
		existingTitles[strings.ToLower(title)] = true
	}
	if logger != nil {
		if created > 0 {
			logging.Fluent(logger).Info("Seeded default policy pack").
				Int("created", created).
				Log()
		} else if len(templates) > 0 {
			logging.Fluent(logger).Info("Default policy pack already satisfied; zero policies created").
				Int("existing", len(existingTitles)).
				Log()
		}
	}
	return created, nil
}

func listExistingPolicyTitles(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) (map[string]bool, error) {
	out := map[string]bool{}
	res, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{Kind: objects.KindPolicy})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return out, nil
	}
	for _, obj := range res.Objects {
		if t := objects.GetString(obj, objects.FieldKeyTitle); t != emptyValue {
			out[strings.ToLower(t)] = true
		}
	}
	return out, nil
}

func loadDefaultPolicyTemplates(projectRoot string) ([]map[string]any, error) {
	candidates := []string{
		filepath.Join(projectRoot, defaultPoliciesRelDir),
	}
	if src := findProjectRootForBootstrap(); src != emptyValue && src != projectRoot {
		candidates = append(candidates, filepath.Join(src, defaultPoliciesRelDir))
	}

	var dir string
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			dir = c
			break
		}
	}
	if dir == emptyValue {
		return embeddedDefaultPolicyTemplates(), nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errfmt.Newf("read default policies dir").Wrap(err)
	}
	var templates []map[string]any
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			return nil, errfmt.Newf("read %s", name).Wrap(rerr)
		}
		var obj map[string]any
		if uerr := yaml.Unmarshal(data, &obj); uerr != nil {
			return nil, errfmt.Newf("parse %s", name).Wrap(uerr)
		}
		templates = append(templates, obj)
	}
	if len(templates) == 0 {
		return embeddedDefaultPolicyTemplates(), nil
	}
	return templates, nil
}

func stableDefaultPolicyID(title string) string {
	sum := sha256.Sum256([]byte("default-policy:" + strings.ToLower(strings.TrimSpace(title))))
	return "POLICY-DEFAULT-" + hex.EncodeToString(sum[:8])
}

// embeddedDefaultPolicyTemplates is a minimal fallback when scripts/default_policies is unavailable.
func embeddedDefaultPolicyTemplates() []map[string]any {
	return []map[string]any{
		{
			objects.FieldKeyTitle:      "Policy Coverage Required (Meta)",
			objects.FieldKeyStatus:     objects.ObjectStatusActive,
			objects.FieldKeyCategory:   "workflow",
			objects.FieldKeyPolicyType: "requirement",
			objects.FieldKeyBody: `This project must maintain an active policy set covering at least these categories:
workflow, agent_guidance, documentation, and (when code exists) code_quality / testing.

Near-term: A default pack is seeded at init. Do not delete the last policy in a required
category without replacing it.

Check: zqk object list policy and verify category coverage before major delivery.`,
			objects.FieldKeyEffectiveDate: "2026-07-25",
		},
		{
			objects.FieldKeyTitle:      "Operational Philosophy - Object-First, Discoverable, Auditable",
			objects.FieldKeyStatus:     objects.ObjectStatusActive,
			objects.FieldKeyCategory:   "workflow",
			objects.FieldKeyPolicyType: "standard",
			objects.FieldKeyBody: `Everything substantive in this system starts from a system object so it can be indexed,
made visible and discoverable, and routinely audited. Work is shaped toward user-defined
mission, vision, and goals.`,
		},
		{
			objects.FieldKeyTitle:      "Multi-Agent Collaboration - Shared Kernel, Shared Constraints",
			objects.FieldKeyStatus:     objects.ObjectStatusActive,
			objects.FieldKeyCategory:   "agent_guidance",
			objects.FieldKeyPolicyType: "requirement",
			objects.FieldKeyBody: `All agents share one knowledge kernel. Orient with zqk workflow whats-next, read active
policies, and create work as objects — not only chat prose.`,
			objects.FieldKeyEffectiveDate: "2026-07-25",
		},
		{
			objects.FieldKeyTitle:      "CLI or MCP for Object Operations",
			objects.FieldKeyStatus:     objects.ObjectStatusActive,
			objects.FieldKeyCategory:   "workflow",
			objects.FieldKeyPolicyType: "standard",
			objects.FieldKeyBody: `Create, update, list, and transition process objects via zqk CLI or MCP.
Do not hand-edit hash YAML under docs/process/ as the primary write path.`,
			objects.FieldKeyEffectiveDate: "2026-07-25",
		},
		{
			objects.FieldKeyTitle:      "For New Agents and Users - Start Here (Tutorial)",
			objects.FieldKeyStatus:     objects.ObjectStatusActive,
			objects.FieldKeyCategory:   "documentation",
			objects.FieldKeyPolicyType: "guideline",
			objects.FieldKeyBody: `Tutorial: read Operational Philosophy policy, then list mission/vision/goals and policies.
Use objects as the indexable source of truth.`,
		},
		{
			objects.FieldKeyTitle:      "Traceable Work - Plans, Backlog, and Relationships",
			objects.FieldKeyStatus:     objects.ObjectStatusActive,
			objects.FieldKeyCategory:   "workflow",
			objects.FieldKeyPolicyType: "requirement",
			objects.FieldKeyBody: `Substantive delivery work must be traceable: prefer an active priority plan with ordered
backlog items; link work to goals/milestones; keep status honest via object updates.`,
			objects.FieldKeyEffectiveDate: "2026-07-25",
		},
		{
			objects.FieldKeyTitle:      "Maintenance Policy - System Behavior Toward Stated Goals",
			objects.FieldKeyStatus:     objects.ObjectStatusActive,
			objects.FieldKeyCategory:   "workflow",
			objects.FieldKeyPolicyType: "standard",
			objects.FieldKeyBody: `Maintenance goals drive efficiency, reliability, robustness, and observability.
Use zqk system init --with-maintenance-jobs (portable maintenance jobs are ensured on init/startup).`,
		},
	}
}
