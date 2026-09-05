package agentprompt

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/skill"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// SyncASKTwins scans the .zqk/skills/ directory and creates missing kernel agent_skill objects.
func SyncASKTwins(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, projectRoot string) error {
	skillsDir := filepath.Join(projectRoot, ".zqk", "skills")
	entries, err := fileutil.ReadDir(skillsDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}

	filter := storage.ListFilter{Kind: objects.KindAgentSkill}
	result, err := sp.List(ctx, secCtx, &pkgctx.StorageContext{}, filter)
	if err != nil {
		return err
	}

	existingByTitle := make(map[string]map[string]any)
	for _, obj := range result.Objects {
		title, _ := obj[objects.FieldKeyTitle].(string)
		if title != "" {
			existingByTitle[title] = obj
		}
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == "SKILL.md" || strings.HasPrefix(name, ".") {
			continue
		}

		title := strings.TrimSuffix(name, ".skill")

		if _, exists := existingByTitle[title]; !exists {
			newObj := map[string]any{
				objects.FieldKeyKind:     objects.KindAgentSkill,
				objects.FieldKeyTitle:    title,
				objects.FieldKeyFilePath: filepath.Join(".zqk", "skills", name),
			}
			err := sp.Create(ctx, secCtx, newObj)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// SkillLoadOptions controls which agent_skill objects are selected for a prompt.
type SkillLoadOptions struct {
	// TaskDescription is free text used for keyword matching (task + plan titles, etc.).
	TaskDescription string
	// PersonaID optionally loads ASK-* ids from persona.related_object_refs.
	PersonaID string
	// ExtraQuery adds role/capability/agent tokens to matching (e.g. TargetAgent, Capability).
	ExtraQuery string
	// MaxInstructionRunes caps embedded instructions per skill (0 = default 6000).
	MaxInstructionRunes int
	// ProjectRoot resolves relative agent_skill file_path values for seal verification.
	ProjectRoot string
}

// SkillEnforcement represents the compiled relevant skills for an agent.
type SkillEnforcement struct {
	RelevantSkills []map[string]any
}

const (
	orchestrationBootMarker = "[orchestration-boot]"
	defaultMaxInstrRunes    = 6000
)

// LoadRelevantSkills fetches agent_skill objects relevant to the given task description.
// Prefer LoadRelevantSkillsOpts when persona / capability context is available.
func LoadRelevantSkills(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, taskDescription string) (*SkillEnforcement, error) {
	return LoadRelevantSkillsOpts(ctx, sp, secCtx, SkillLoadOptions{TaskDescription: taskDescription})
}

// LoadRelevantSkillsOpts selects skills via persona-linked ASK refs, orchestration-boot
// markers, and keyword matching against title/summary/description tokens.
func LoadRelevantSkillsOpts(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, opt SkillLoadOptions) (*SkillEnforcement, error) {
	if sp == nil {
		return &SkillEnforcement{}, nil
	}
	storageCtx := &pkgctx.StorageContext{}

	filter := storage.ListFilter{
		Kind:  objects.KindAgentSkill,
		Limit: 0,
	}
	result, err := sp.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to list agent skills from kernel").Wrap(err)
	}

	wantIDs := map[string]struct{}{}
	if pid := strings.TrimSpace(opt.PersonaID); pid != "" && strings.HasPrefix(strings.ToUpper(pid), "PER-") {
		persona, err := sp.Read(ctx, secCtx, pid)
		if err == nil && persona != nil {
			for _, id := range relatedASKRefs(persona) {
				wantIDs[id] = struct{}{}
			}
		}
	}

	query := strings.ToLower(strings.TrimSpace(opt.TaskDescription + " " + opt.ExtraQuery))
	var relevant []map[string]any
	seen := map[string]struct{}{}

	appendSkill := func(skillObj map[string]any) error {
		id, _ := skillObj[objects.FieldKeyID].(string)
		if id == "" {
			return nil
		}
		if _, ok := seen[id]; ok {
			return nil
		}
		st, _ := skillObj[objects.FieldKeyStatus].(string)
		st = strings.ToLower(strings.TrimSpace(st))
		if st == "archived" || st == "error" {
			return nil
		}

		path, _ := skillObj[objects.FieldKeyFilePath].(string)
		if path != "" {
			skillMdPath := resolveSkillMarkdownPath(opt.ProjectRoot, path)
			content, err := fileutil.ReadFile(skillMdPath)
			if err != nil {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logger.Error("Failed to read skill file for verification", err, logging.String("skill", id), logging.String("path", skillMdPath))
				// TRACK: REDACTED — fail-closed when file_path is set.
				return fmt.Errorf("skill %s failed seal verification: cannot read %s: %w", id, skillMdPath, err)
			}
			verifyRes := skill.VerifySeal(string(content))
			if !verifyRes.Valid {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logger.Error("Skill seal verification failed", fmt.Errorf("%s", verifyRes.Error), logging.String("skill", id), logging.String("path", skillMdPath))
				return fmt.Errorf("skill %s failed seal verification: %s", id, verifyRes.Error)
			}
		}

		seen[id] = struct{}{}
		relevant = append(relevant, skillObj)
		return nil
	}

	var bootASKs []string
	for _, skillObj := range result.Objects {
		id, _ := skillObj[objects.FieldKeyID].(string)
		summary, _ := skillObj[objects.FieldKeyInstructionsSummary].(string)
		title, _ := skillObj[objects.FieldKeyTitle].(string)
		desc, _ := skillObj[objects.FieldKeyDescription].(string)

		if _, ok := wantIDs[id]; ok {
			if err := appendSkill(skillObj); err != nil {
				return nil, err
			}
			continue
		}
		if strings.Contains(strings.ToLower(summary), orchestrationBootMarker) {
			if err := appendSkill(skillObj); err != nil {
				return nil, err
			}
			if id != "" {
				bootASKs = append(bootASKs, id)
			}
			continue
		}
		if query == "" {
			continue
		}
		if skillMatchesQuery(query, title, desc, summary) {
			if err := appendSkill(skillObj); err != nil {
				return nil, err
			}
		}
	}

	if pid := strings.TrimSpace(opt.PersonaID); pid != "" && len(bootASKs) > 0 {
		persona, err := sp.Read(ctx, secCtx, pid)
		if err == nil && persona != nil {
			changed := false
			existingMap := make(map[string]bool)
			for _, ref := range relatedASKRefs(persona) {
				existingMap[ref] = true
			}

			var newRefs []any
			if raw, ok := persona[objects.FieldKeyRelatedObjectRefs].([]any); ok {
				newRefs = raw
			} else if rawStr, ok := persona[objects.FieldKeyRelatedObjectRefs].([]string); ok {
				for _, s := range rawStr {
					newRefs = append(newRefs, s)
				}
			}

			for _, bootASK := range bootASKs {
				if !existingMap[bootASK] {
					newRefs = append(newRefs, bootASK)
					changed = true
				}
			}

			if changed {
				persona[objects.FieldKeyRelatedObjectRefs] = newRefs
				_ = sp.Update(ctx, secCtx, pid, persona)
			}
		}
	}

	return &SkillEnforcement{RelevantSkills: relevant}, nil
}

func relatedASKRefs(persona map[string]any) []string {
	return objects.CollectPersonaASKRefs(persona)
}

// resolveSkillMarkdownPath joins projectRoot for relative paths and maps skill
// directories to SKILL.md. Paths that already end in .md are used as-is.
func resolveSkillMarkdownPath(projectRoot, path string) string {
	skillMdPath := strings.TrimSpace(path)
	if skillMdPath == "" {
		return skillMdPath
	}
	if !filepath.IsAbs(skillMdPath) && projectRoot != "" {
		skillMdPath = filepath.Join(projectRoot, skillMdPath)
	}
	lower := strings.ToLower(skillMdPath)
	if strings.HasSuffix(lower, ".md") {
		return skillMdPath
	}
	if st, err := fileutil.Stat(skillMdPath); err == nil && !st.IsDir() {
		return skillMdPath
	}
	return filepath.Join(skillMdPath, "SKILL.md")
}

func skillMatchesQuery(query, title, desc, summary string) bool {
	title = strings.ToLower(strings.TrimSpace(title))
	desc = strings.ToLower(strings.TrimSpace(desc))
	summary = strings.ToLower(strings.TrimSpace(summary))
	if title != "" && strings.Contains(query, title) {
		return true
	}
	if desc != "" && strings.Contains(query, desc) {
		return true
	}
	if summary != "" && strings.Contains(query, summary) {
		return true
	}
	// Token overlap: any title word ≥4 chars appears in the query.
	for _, tok := range strings.FieldsFunc(title, func(r rune) bool {
		return r == ' ' || r == '-' || r == '/' || r == '_' || r == ':' || r == ','
	}) {
		tok = strings.TrimSpace(tok)
		if len(tok) < 4 {
			continue
		}
		if strings.Contains(query, tok) {
			return true
		}
	}
	return false
}

// GeneratePromptSection lists skills as ID+title+path refs. Bodies stay on ASK-* / SKILL.md.
func (s *SkillEnforcement) GeneratePromptSection() string {
	return s.GeneratePromptSectionRefs()
}

// GeneratePromptSectionRefs is the default skill section: links, not copied instruction bodies.
func (s *SkillEnforcement) GeneratePromptSectionRefs() string {
	if len(s.RelevantSkills) == 0 {
		return "## Relevant Agent Skills\nNo specialized skills discovered for this task.\n"
	}

	var sb strings.Builder
	sb.WriteString("## Relevant Agent Skills\n")
	sb.WriteString("Resolve mandates with `zqk object get <ASK-id>` or the listed `file_path`. Do not persist instruction bodies on the task object.\n\n")

	for _, skillObj := range s.RelevantSkills {
		title, _ := skillObj[objects.FieldKeyTitle].(string)
		id, _ := skillObj[objects.FieldKeyID].(string)
		summary, _ := skillObj[objects.FieldKeyInstructionsSummary].(string)
		path, _ := skillObj[objects.FieldKeyFilePath].(string)
		sb.WriteString(fmt.Sprintf("- `%s` — %s\n", id, title))
		if path != "" {
			sb.WriteString(fmt.Sprintf("  - path: `%s`\n", path))
		}
		if summary != "" {
			sb.WriteString(fmt.Sprintf("  - summary: %s\n", summary))
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// GeneratePromptSectionOpts inlines instruction bodies. Do not persist this on agent_task.
func (s *SkillEnforcement) GeneratePromptSectionOpts(maxInstrRunes int) string {
	if len(s.RelevantSkills) == 0 {
		return "## Relevant Agent Skills\nNo specialized skills discovered for this task.\n"
	}
	if maxInstrRunes <= 0 {
		maxInstrRunes = defaultMaxInstrRunes
	}

	var sb strings.Builder
	sb.WriteString("## Relevant Agent Skills\n")
	sb.WriteString("The following specialized project skills are binding for this task. Follow their mandates; narrative claims are not a substitute for evidence gates (VDS / scan-tests).\n\n")

	for _, skill := range s.RelevantSkills {
		title, _ := skill[objects.FieldKeyTitle].(string)
		desc, _ := skill[objects.FieldKeyDescription].(string)
		id, _ := skill[objects.FieldKeyID].(string)
		summary, _ := skill[objects.FieldKeyInstructionsSummary].(string)
		instr, _ := skill[objects.FieldKeyInstructions].(string)
		path, _ := skill[objects.FieldKeyFilePath].(string)

		sb.WriteString(fmt.Sprintf("### %s (%s)\n", title, id))
		if path != "" {
			sb.WriteString(fmt.Sprintf("- **Location**: `%s`\n", path))
		}
		if summary != "" {
			sb.WriteString(fmt.Sprintf("- **Summary**: %s\n", summary))
		}
		if desc != "" {
			sb.WriteString(fmt.Sprintf("- **Focus**: %s\n", desc))
		}
		if instr != "" {
			sb.WriteString("\n#### Mandates\n\n")
			sb.WriteString(truncateRunes(instr, maxInstrRunes))
			sb.WriteString("\n")
		} else {
			sb.WriteString("\n_No instructions body on this agent_skill — treat as incomplete registration._\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n\n…(instructions truncated for prompt budget)…"
}
