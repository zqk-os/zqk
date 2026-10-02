package system

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/bootstrap"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	defaultPersonasRelDir = "scripts/default_personas"
	defaultSkillsRelDir   = "scripts/default_agent_skills"
)

// SeedDefaultAgentSeatingPack creates community-default personas and agent_skills
// after greenfield init so feed/chat works without Studio-only PER-* IDs.
// Idempotent: skips when the stable object id already exists.
func SeedDefaultAgentSeatingPack(projectRoot string, logger logging.Logger) (created int, err error) {
	// Promote-ready status (approved) must land on CAS — not draft plane.
	// Draft-only seating made feed ack/steer resolve "object not found" for PER-DEFAULT-*.
	ctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	sp, secCtx, err := initSeedingStorage(ctx, projectRoot, "default agent seating")
	if err != nil {
		return 0, err
	}
	if storage.GetCacheOperationHandler() == nil {
		storage.SetCacheOperationHandler(func(cacheCtx *pkgctx.CacheContext) error {
			if cacheCtx.Operation == pkgctx.CacheOperationUpdate || cacheCtx.Operation == pkgctx.CacheOperationInvalidateAndUpdate {
				_ = UpdateObjectIDCache(cacheCtx.NewID, cacheCtx.Kind, cacheCtx.FilePath)
			}
			return nil
		})
	}

	// Ensure zqk-expert skill pack files exist in .zqk/skills and .agent/skills
	if serr := EnsureDefaultSkillsFiles(projectRoot, logger); serr != nil {
		if logger != nil {
			logging.Fluent(logger).Warn("Could not ensure default skill files").WithError(serr).Log()
		}
	}

	personaTmpls, perr := loadYAMLTemplates(projectRoot, defaultPersonasRelDir, embeddedDefaultPersonaTemplates)
	if perr != nil {
		return 0, perr
	}
	skillTmpls, serr := loadYAMLTemplates(projectRoot, defaultSkillsRelDir, embeddedDefaultAgentSkillTemplates)
	if serr != nil {
		return 0, serr
	}

	now := time.Now().UTC().Format(time.RFC3339)
	n, err := seedKindTemplates(ctx, sp, secCtx, personaTmpls, objects.KindPersona, "approved", now, logger)
	if err != nil {
		return created, err
	}
	created += n
	n, err = seedKindTemplates(ctx, sp, secCtx, skillTmpls, objects.KindAgentSkill, "approved", now, logger)
	if err != nil {
		return created, err
	}
	created += n

	if logger != nil {
		if created > 0 {
			logging.Fluent(logger).Info("Seeded default agent seating pack").
				Int("created", created).
				Log()
		} else {
			logging.Fluent(logger).Info("Default agent seating pack already satisfied; zero objects created").
				Log()
		}
	}
	return created, nil
}

func seedKindTemplates(
	ctx context.Context,
	sp storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	templates []map[string]any,
	kind string,
	defaultStatus string,
	now string,
	logger logging.Logger,
) (int, error) {
	created := 0
	for _, tmpl := range templates {
		obj := map[string]any{}
		for k, v := range tmpl {
			obj[k] = v
		}
		obj[objects.FieldKeyKind] = kind
		id := objects.GetString(obj, objects.FieldKeyID)
		if id == emptyValue {
			title := objects.GetString(obj, objects.FieldKeyTitle)
			if title == emptyValue {
				continue
			}
			return created, errfmt.Errorf("default %s template missing id (title=%q)", kind, title)
		}
		if existing, rerr := sp.Read(ctx, secCtx, id); rerr == nil && existing != nil {
			continue
		}
		if objects.GetString(obj, objects.FieldKeySchemaVersion) == emptyValue {
			obj[objects.FieldKeySchemaVersion] = objects.DefaultSchemaVersion
		}
		if objects.GetString(obj, objects.FieldKeyStatus) == emptyValue {
			obj[objects.FieldKeyStatus] = defaultStatus
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
			return created, errfmt.Newf("create default %s %s", kind, id).Wrap(cerr)
		}
		created++
		if logger != nil {
			logging.Fluent(logger).Debug("Created default seating object").
				ObjectID(id).
				String("kind", kind).
				Log()
		}
	}
	return created, nil
}

func loadYAMLTemplates(projectRoot, relDir string, embedded func() []map[string]any) ([]map[string]any, error) {
	candidates := []string{
		filepath.Join(projectRoot, relDir),
	}
	if src := bootstrap.FindSourceProjectRoot(); src != emptyValue && src != projectRoot {
		candidates = append(candidates, filepath.Join(src, relDir))
	}

	var dir string
	for _, c := range candidates {
		if st, err := fileutil.Stat(c); err == nil && st.IsDir() {
			dir = c
			break
		}
	}
	if dir == emptyValue {
		return embedded(), nil
	}

	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return nil, errfmt.Newf("read %s", relDir).Wrap(err)
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
		data, rerr := fileutil.ReadFile(filepath.Join(dir, name))
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
		return embedded(), nil
	}
	return templates, nil
}

func embeddedDefaultPersonaTemplates() []map[string]any {
	return []map[string]any{
		{
			objects.FieldKeyID:          objects.ConstPersonaDefaultOperator,
			objects.FieldKeyName:        "Community Operator",
			objects.FieldKeyTitle:       "Community Operator",
			objects.FieldKeyRole:        "operator",
			objects.FieldKeyStatus:      objects.ObjectStatusApproved,
			objects.FieldKeyDescription: "Default human/operator seating for agent correspondence after greenfield init.",
		},
		{
			objects.FieldKeyID:          objects.ConstPersonaDefaultAgent,
			objects.FieldKeyName:        "Community Agent",
			objects.FieldKeyTitle:       "Community Agent",
			objects.FieldKeyRole:        "agent",
			objects.FieldKeyStatus:      objects.ObjectStatusApproved,
			objects.FieldKeyDescription: "Default peer-agent seating for multi-agent correspondence after greenfield init.",
		},
	}
}

func embeddedDefaultAgentSkillTemplates() []map[string]any {
	return []map[string]any{
		{
			objects.FieldKeyID:                  "ASK-DEFAULT-FEED-CORRESPONDENCE",
			objects.FieldKeyTitle:               "Agent Feed Correspondence (Community Default)",
			objects.FieldKeyStatus:              objects.ObjectStatusApproved,
			objects.FieldKeyProvider:            "zqk",
			objects.FieldKeyInstructionsSummary: paths.RewriteCanonicalCLIInvocations("Use kernel personas and zqk feed for out-of-the-box multi-agent chat."),
			objects.FieldKeyInstructions: paths.RewriteCanonicalCLIInvocations(fmt.Sprintf(`# Agent Feed Correspondence

After init, use %s / %s with zqk feed steer and emit-status --persona-ref.
Do not invent role enums; agent-id is the unique swarm seat.`, objects.ConstPersonaDefaultOperator, objects.ConstPersonaDefaultAgent)),
		},
		{
			objects.FieldKeyID:                  "ASK-DEFAULT-ZQK-EXPERT",
			objects.FieldKeyTitle:               "ZQK Expert Operating Protocol (Community Default)",
			objects.FieldKeyStatus:              objects.ObjectStatusApproved,
			objects.FieldKeyProvider:            "zqk",
			objects.FieldKeyInstructionsSummary: paths.RewriteCanonicalCLIInvocations("Expert guidance for Knowledge Kernel operations, CLI-only process data, VDS done-gates, and fail-closed discipline."),
			objects.FieldKeyInstructions: paths.RewriteCanonicalCLIInvocations(`# ZQK Expert Operating Protocol

## Objective
Govern Knowledge Kernel operations, autonomous execution loops, fail-closed compliance,
and verifiable done-gates (VDS).

## Operating Directives
1. **Kernel vs Prompting**: Anchor all work in typed, content-addressed kernel objects under .zqk/process. Never edit CAS YAML by hand.
2. **Autonomous Execution (zqk do)**: Use single-command loop zqk do <bli_id> to claim and verify work atomically.
3. **VDS Done-Gates**: Mark backlog items complete only after running deterministic verification (zqk do <id> --verify or zqk workflow vds evaluate).
4. **Fail-Closed Operations**: If any verification or schema check fails, emit explicit failure context and halt. Narrative hand-waving is strictly forbidden.
5. **Timeline & Gantt**: Workstreams are long-lived reporting lanes, priority plans are scoped execution bundles, milestones provide temporal anchors, and backlog items are atomic tasks.`),
		},
	}
}

// SeedKernelFromAnswerFile reads an answer file (YAML array or single object or dict of objects)
// and seeds the declarative system objects into kernel storage during init.
func SeedKernelFromAnswerFile(projectRoot, answerFilePath string, logger logging.Logger) (int, error) {
	if projectRoot == emptyValue || answerFilePath == emptyValue {
		return 0, nil
	}
	data, err := fileutil.ReadFile(answerFilePath)
	if err != nil {
		return 0, errfmt.Newf("read answer file %s", answerFilePath).Wrap(err)
	}

	var items []map[string]any
	var single map[string]any
	if err := yaml.Unmarshal(data, &items); err != nil || len(items) == 0 {
		if err := yaml.Unmarshal(data, &single); err == nil && len(single) > 0 {
			if objects.GetString(single, objects.FieldKeyKind) != emptyValue {
				items = []map[string]any{single}
			} else if rawObjs, ok := single["objects"].([]any); ok {
				for _, ro := range rawObjs {
					if m, ok := ro.(map[string]any); ok {
						items = append(items, m)
					}
				}
			}
		}
	}

	if len(items) == 0 {
		return 0, nil
	}

	ctx := pkgctx.NewSystemContext()
	sp, secCtx, err := initSeedingStorage(ctx, projectRoot, "answer file seed")
	if err != nil {
		return 0, err
	}

	created := 0
	var lastCreateErr error
	for _, item := range items {
		kind := objects.GetString(item, objects.FieldKeyKind)
		id := objects.GetString(item, objects.FieldKeyID)
		if kind == emptyValue || id == emptyValue {
			continue
		}
		if existing, _ := sp.Read(ctx, secCtx, id); existing != nil {
			continue
		}
		// Create at lifecycle origin. The answer file's status is often a kind-blind
		// "active" that the kind never grants (persona) or a two-hop target (policy).
		createStatus, leaveStatus := seedAnswerFileVisibilityStatuses(kind)
		if createStatus != emptyValue {
			item[objects.FieldKeyStatus] = createStatus
		}
		if err := sp.Create(ctx, secCtx, item); err != nil {
			lastCreateErr = err
			continue
		}
		// answer-file seed must promote off draft plane for CAS visibility.
		if leaveStatus != emptyValue && leaveStatus != createStatus {
			if err := sp.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyStatus: leaveStatus}); err != nil {
				if _, readErr := sp.Read(ctx, secCtx, id); readErr != nil {
					continue
				}
			}
		}
		created++
	}
	if logger != nil && created > 0 {
		logging.Fluent(logger).Info("Seeded kernel objects from answer file").
			Int("created", created).
			String("answer_file", answerFilePath).
			Log()
	}
	if created == 0 && lastCreateErr != nil {
		return 0, errfmt.Newf("answer file seed created 0 objects").Wrap(lastCreateErr)
	}
	return created, nil
}

// seedAnswerFileVisibilityStatuses returns the origin status for Create and the
// first non-preliminary promote hop for leave. A single Update to the answer
// file's intended status is not enough: leave==create is a no-op (draft plane)
// and kind-blind "active" is often not in the lifecycle at all.
func seedAnswerFileVisibilityStatuses(kind string) (createStatus, leaveStatus string) {
	if kind == emptyValue {
		return "", ""
	}
	loader := objects.NewLifecycleLoader("")
	origin, err := loader.GetOriginStatus(kind)
	if err != nil || origin == emptyValue {
		return "", ""
	}
	return origin, firstNonPreliminaryPromoteLeave(loader, kind, origin)
}

func firstNonPreliminaryPromoteLeave(loader *objects.LifecycleLoader, kind, origin string) string {
	if loader == nil || kind == emptyValue || origin == emptyValue {
		return origin
	}
	lc, err := loader.LoadLifecycle(kind)
	if err != nil || lc == nil {
		return origin
	}
	targets := objects.PromoteTransitionTargets(lc, origin)
	names := make([]string, 0, len(targets))
	for to := range targets {
		if to == objects.ObjectStatusArchived || to == objects.ObjectStatusError {
			continue
		}
		names = append(names, to)
	}
	slices.Sort(names)
	for _, to := range names {
		prelim, pErr := loader.IsPreliminaryStatusForKind(kind, to)
		if pErr == nil && !prelim {
			return to
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return origin
}
