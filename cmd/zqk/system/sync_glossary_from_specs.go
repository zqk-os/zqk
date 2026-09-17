package system

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/internal/cli"
	bldr_cli_cmd_v1 "github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	glossary_term "github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1/glossary_terms"
	bldr_instance_v1 "github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
)

type glossaryCandidate struct {
	Title       string `json:"title" yaml:"title"`
	Category    string `json:"category" yaml:"category"`
	Context     string `json:"context_scope" yaml:"context_scope"`
	Definition  string `json:"definition" yaml:"definition"`
	AgentPrompt string `json:"agent_prompts" yaml:"agent_prompts"`
	MachineHint string `json:"machine_hints" yaml:"machine_hints"`
	SourceType  string `json:"source_type" yaml:"source_type"`
	SourcePath  string `json:"source_path" yaml:"source_path"`
}

type syncGlossaryFromSpecsReport struct {
	DryRun             bool                `json:"dry_run" yaml:"dry_run"`
	SpecsDir           string              `json:"specs_dir" yaml:"specs_dir"`
	LifecyclesDir      string              `json:"lifecycles_dir" yaml:"lifecycles_dir"`
	ConfigsDir         string              `json:"configs_dir" yaml:"configs_dir"`
	ExistingGLS        int                 `json:"existing_glossary_terms" yaml:"existing_glossary_terms"`
	DetectedCandidates int                 `json:"detected_candidates" yaml:"detected_candidates"`
	MissingCandidates  int                 `json:"missing_candidates" yaml:"missing_candidates"`
	CreatedCount       int                 `json:"created_count" yaml:"created_count"`
	SkippedDupIngest   int                 `json:"skipped_duplicate_ingest" yaml:"skipped_duplicate_ingest"`
	Missing            []glossaryCandidate `json:"missing" yaml:"missing"`
	Created            []glossaryCandidate `json:"created,omitempty" yaml:"created,omitempty"`
}

// JSON keys inside glossary_term.machine_hints used to dedupe sync-from-specs ingest (aligned with scan*Candidates hints).
const (
	ingestHintKeySpecPath      = "spec_path"
	ingestHintKeyLifecyclePath = "lifecycle_path"
	ingestHintKeyConfigPath    = "config_path"
)

// NewSyncGlossaryFromSpecsCmd wires command builder from spec:
// .zqk/cli/specs/system/sync_glossary_from_specs_command.yaml
func NewSyncGlossaryFromSpecsCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemSyncGlossaryFromSpecsCommandBuilder()
	cmd.Args = cobra.NoArgs
	cmd.RunE = runSyncGlossaryFromSpecs
	return cmd
}

func runSyncGlossaryFromSpecs(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var err error
		_ = err
		root := proc.ProjectRoot()
		if root == emptyValue {
			return errfmt.Errorf("project root not found")
		}
		specsDir, _ := cmd.Flags().GetString("specs-dir")
		commandSpecsDir, _ := cmd.Flags().GetString("command-specs-dir")
		lifecyclesDir, _ := cmd.Flags().GetString("lifecycles-dir")
		configsDir, _ := cmd.Flags().GetString("configs-dir")
		if specsDir == emptyValue {
			specsDir = paths.ProcessInternalObjectSpecsDir
		}
		if commandSpecsDir == emptyValue {
			commandSpecsDir = paths.CLICommandSpecsDir
		}
		if lifecyclesDir == emptyValue {
			lifecyclesDir = paths.ProcessInternalLifecyclesDir
		}
		if configsDir == emptyValue {
			configsDir = paths.ProcessInternalConfigsDir
		}
		specsDir = toAbs(root, specsDir)
		commandSpecsDir = toAbs(root, commandSpecsDir)
		lifecyclesDir = toAbs(root, lifecyclesDir)
		configsDir = toAbs(root, configsDir)

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		apply, _ := cmd.Flags().GetBool("apply")

		candidates, err := collectGlossaryCandidates(root, specsDir, commandSpecsDir, lifecyclesDir, configsDir)
		if err != nil {
			return err
		}
		titles, sourceKeys, err := loadGlossaryIngestDedupeIndex(proc.Storage(), cmd)
		if err != nil {
			return err
		}
		missing := make([]glossaryCandidate, 0, len(candidates))
		for _, c := range candidates {
			if sk := candidateIngestSourceKey(c); sk != "" {
				if _, ok := sourceKeys[sk]; ok {
					continue
				}
			}
			if _, ok := titles[c.Title]; !ok {
				missing = append(missing, c)
			}
		}
		slices.SortFunc(missing, func(a, b glossaryCandidate) int { return strings.Compare(a.Title, b.Title) })

		created := 0
		skippedDup := 0
		createdList := make([]glossaryCandidate, 0)
		if apply && !dryRun {
			for _, c := range missing {
				skipped, err := createGlossaryCandidate(cmd, proc.Storage(), c, titles, sourceKeys)
				if err != nil {
					return err
				}
				if skipped {
					skippedDup++
					continue
				}
				created++
				createdList = append(createdList, c)
			}
			if created > 0 {
				flushCtx, cancelFlush := storage.DurabilityFlushContext()
				defer cancelFlush()
				if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), []string{objects.KindGlossaryTerm}); err != nil {
					logging.FluentEvent(proc.Logger()).Warn("Persist flush after glossary sync timed out").WithError(err).Log()
				}
			}
		}

		// After apply, do not re-report the pre-create set as "missing" — that made builds look
		// like the same candidates failed to persist when created_count > 0.
		reportMissing := missing
		missingCount := len(missing)
		if apply && !dryRun {
			reportMissing = nil
			missingCount = 0
		}

		rep := syncGlossaryFromSpecsReport{
			DryRun:             dryRun || !apply,
			SpecsDir:           specsDir,
			LifecyclesDir:      lifecyclesDir,
			ConfigsDir:         configsDir,
			ExistingGLS:        len(titles),
			DetectedCandidates: len(candidates),
			MissingCandidates:  missingCount,
			CreatedCount:       created,
			SkippedDupIngest:   skippedDup,
			Missing:            reportMissing,
			Created:            createdList,
		}
		return outputSyncGlossaryReport(cmd, rep)
	})(cmd, nil)
}

// newCASGlossaryTermID allocates an ID using the same shape as FileObjectStorage.ensureObjectID for
// CAS-backed kinds (timestamp + random hex). Sequential id_generation cannot observe existing IDs in
// hash-based CAS layouts and collides with legacy numeric test IDs (e.g. GLS-001).
func newCASGlossaryTermID() (string, error) {
	v := validation.GetIDValidator()
	if err := v.LoadPatterns(); err != nil {
		return "", errfmt.Newf("load id patterns").Wrap(err)
	}
	prefixes := v.GetValidPrefixes(objects.KindGlossaryTerm)
	if len(prefixes) == 0 {
		return "", errfmt.Errorf("no valid ID prefix for kind %s", objects.KindGlossaryTerm)
	}
	prefix := prefixes[0]
	baseTime := time.Now().UnixNano()
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("%s%d", prefix, baseTime), nil
	}
	return fmt.Sprintf("%s%d-%s", prefix, baseTime, hex.EncodeToString(randomBytes)), nil
}

// createGlossaryCandidate persists one glossary_term for a missing candidate. Returns (true, nil) when the
// ingest preflight finds an existing row with the same title or same spec/lifecycle/config source path
// (fresh index read) so callers can count skips separately from creates.
func createGlossaryCandidate(cmd *cobra.Command, sp storage.ObjectStorageProvider, c glossaryCandidate, optMaps ...map[string]struct{}) (skipped bool, err error) {
	// Tactical: allocate IDs here so --apply works on greenfield. Must match CAS ID allocation in
	// storage (see newCASGlossaryTermID). Long-term: data-cell steward; see SPEC_ORIGIN_PLANE glossary.
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return false, errfmt.Newf("processor").Wrap(err)
	}
	if proc.ProjectRoot() == emptyValue {
		return false, errfmt.Errorf("project root not found")
	}
	var titles map[string]struct{}
	var sourceKeys map[string]struct{}
	if len(optMaps) >= 2 {
		titles = optMaps[0]
		sourceKeys = optMaps[1]
	}
	if titles == nil || sourceKeys == nil {
		var err error
		titles, sourceKeys, err = loadGlossaryIngestDedupeIndex(sp, cmd)
		if err != nil {
			return false, errfmt.Newf("load glossary ingest dedupe index").Wrap(err)
		}
	}
	title := strings.TrimSpace(c.Title)
	if title != emptyValue {
		if _, ok := titles[title]; ok {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Info("sync-glossary-from-specs: skipping ingest duplicate (title already exists)").
				String("title", title).
				String("source_path", c.SourcePath).
				String("source_type", c.SourceType).
				Log()
			return true, nil
		}
	}
	if sk := candidateIngestSourceKey(c); sk != "" {
		if _, ok := sourceKeys[sk]; ok {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Info("sync-glossary-from-specs: skipping ingest duplicate (source path already ingested)").
				String("title", title).
				String("source_path", sk).
				String("source_type", c.SourceType).
				Log()
			return true, nil
		}
	}
	newID, err := newCASGlossaryTermID()
	if err != nil {
		return false, errfmt.Errorf("generate glossary_term id: %w", err)
	}

	b := bldr_instance_v1.NewGlossaryTermInstanceBuilder(objects.DefaultSchemaVersion).
		ID(newID).
		Title(c.Title).
		ContextScope(c.Context).
		Category(c.Category).
		Definition(c.Definition).
		AgentPrompts(c.AgentPrompt).
		MachineHints(c.MachineHint).
		SourceType("internal").
		OriginProject("zqk").
		OriginSystem("zqk").
		Status(glossary_term.StatusActive)

	instance, err := b.Build()
	if err != nil {
		return false, errfmt.Errorf("build glossary_term %q: %w", c.Title, err)
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	createCtx := pkgctx.WithPromoteOnCreate(cmd.Context())
	// Let write-behind handle durability asynchronously inside the loop;
	// runSyncGlossaryFromSpecs will perform a single, final flush before CLI exit.
	if err := sp.Create(createCtx, secCtx, instance); err != nil {
		return false, errfmt.Errorf("create glossary_term %q: %w", c.Title, err)
	}
	if titles != nil {
		title := strings.TrimSpace(c.Title)
		if title != emptyValue {
			titles[title] = struct{}{}
		}
	}
	if sourceKeys != nil {
		if sk := candidateIngestSourceKey(c); sk != "" {
			sourceKeys[sk] = struct{}{}
		}
	}
	return false, nil
}

func outputSyncGlossaryReport(cmd *cobra.Command, rep syncGlossaryFromSpecsReport) error {
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, rep)
	default:
		var b strings.Builder
		b.WriteString("sync-glossary-from-specs\n")
		b.WriteString("------------------------\n")
		fmt.Fprintf(&b, "dry_run: %v\n", rep.DryRun)
		fmt.Fprintf(&b, "existing_glossary_terms: %d\n", rep.ExistingGLS)
		fmt.Fprintf(&b, "detected_candidates: %d\n", rep.DetectedCandidates)
		fmt.Fprintf(&b, "missing_candidates: %d\n", rep.MissingCandidates)
		if rep.CreatedCount > 0 {
			fmt.Fprintf(&b, "created_count: %d\n", rep.CreatedCount)
		}
		if rep.SkippedDupIngest > 0 {
			fmt.Fprintf(&b, "skipped_duplicate_ingest: %d\n", rep.SkippedDupIngest)
		}
		if len(rep.Created) > 0 {
			b.WriteString("\nCreated glossary terms:\n")
			for _, c := range rep.Created {
				fmt.Fprintf(&b, "  - %s (%s) [%s]\n", c.Title, c.SourceType, c.SourcePath)
			}
		}
		if len(rep.Missing) > 0 {
			b.WriteString("\nMissing glossary candidates:\n")
			for _, c := range rep.Missing {
				fmt.Fprintf(&b, "  - %s (%s) [%s]\n", c.Title, c.SourceType, c.SourcePath)
			}
			// Dry-run only: do not re-print this after --apply (build/codegen already persists).
			if rep.DryRun {
				b.WriteString("\nApply:\n")
				b.WriteString("  zqk-admin system sync-glossary-from-specs --apply --dry-run=false\n")
			}
		}
		return cli.WriteOutput(cmd, []byte(b.String()))
	}
}

func listExistingGlossaryTitles(sp storage.ObjectStorageProvider, cmd *cobra.Command) (map[string]struct{}, error) {
	titles, _, err := loadGlossaryIngestDedupeIndex(sp, cmd)
	return titles, err
}

// loadGlossaryIngestDedupeIndex returns (titles, sourcePathKeys) from a single List pass. sourcePathKeys
// are normalized paths from machine_hints (spec_path, lifecycle_path, config_path) for ingest deduplication.
func loadGlossaryIngestDedupeIndex(sp storage.ObjectStorageProvider, cmd *cobra.Command) (map[string]struct{}, map[string]struct{}, error) {
	if sp == nil {
		return nil, nil, errfmt.Errorf("storage not available")
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	res, err := sp.List(cmd.Context(), secCtx, storageCtx, storage.ListFilter{Kind: objects.KindGlossaryTerm, Limit: 0})
	if err != nil {
		return nil, nil, errfmt.Newf("list glossary_term").Wrap(err)
	}
	titles := make(map[string]struct{})
	sourceKeys := make(map[string]struct{})
	for _, obj := range res.Objects {
		title, _ := obj[objects.FieldKeyTitle].(string)
		title = strings.TrimSpace(title)
		if title != emptyValue {
			titles[title] = struct{}{}
		}
		ingestSourceKeysFromGlossaryObject(obj, sourceKeys)
	}
	return titles, sourceKeys, nil
}

func normalizeIngestSourcePath(s string) string {
	s = strings.TrimSpace(s)
	// machine_hints paths may contain backslashes from tooling; normalize for stable dedupe keys.
	s = strings.ReplaceAll(s, `\`, `/`)
	return filepath.ToSlash(s)
}

func candidateIngestSourceKey(c glossaryCandidate) string {
	return normalizeIngestSourcePath(c.SourcePath)
}

func ingestSourceKeysFromGlossaryObject(obj map[string]any, out map[string]struct{}) {
	raw, _ := obj[objects.FieldKeyMachineHints].(string)
	if strings.TrimSpace(raw) == emptyValue {
		return
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return
	}
	for _, k := range []string{ingestHintKeySpecPath, ingestHintKeyLifecyclePath, ingestHintKeyConfigPath} {
		if v, ok := m[k].(string); ok {
			if nk := normalizeIngestSourcePath(v); nk != emptyValue {
				out[nk] = struct{}{}
			}
		}
	}
}

func collectGlossaryCandidates(root, specsDir, commandSpecsDir, lifecyclesDir, configsDir string) ([]glossaryCandidate, error) {
	out := make([]glossaryCandidate, 0, 256)
	seen := map[string]struct{}{}

	specCandidates, err := scanSpecCandidates(root, specsDir)
	if err != nil {
		return nil, err
	}
	for _, c := range specCandidates {
		if _, ok := seen[c.Title]; ok {
			continue
		}
		seen[c.Title] = struct{}{}
		out = append(out, c)
	}
	commandSpecCandidates, err := scanCommandSpecCandidates(root, commandSpecsDir)
	if err != nil {
		return nil, err
	}
	for _, c := range commandSpecCandidates {
		if _, ok := seen[c.Title]; ok {
			continue
		}
		seen[c.Title] = struct{}{}
		out = append(out, c)
	}
	lifecycleCandidates, err := scanLifecycleCandidates(root, lifecyclesDir)
	if err != nil {
		return nil, err
	}
	for _, c := range lifecycleCandidates {
		if _, ok := seen[c.Title]; ok {
			continue
		}
		seen[c.Title] = struct{}{}
		out = append(out, c)
	}
	configCandidates, err := scanConfigCandidates(root, configsDir)
	if err != nil {
		return nil, err
	}
	for _, c := range configCandidates {
		if _, ok := seen[c.Title]; ok {
			continue
		}
		seen[c.Title] = struct{}{}
		out = append(out, c)
	}
	return out, nil
}

func scanCommandSpecCandidates(root, dir string) ([]glossaryCandidate, error) {
	out := make([]glossaryCandidate, 0, 128)
	err := filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		m, err := readYAMLMap(path)
		if err != nil {
			return errfmt.Errorf("read command spec %s: %w", path, err)
		}
		name := strings.TrimSpace(strFromAny(m[objects.FieldKeyName]))
		if name == emptyValue {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err == nil {
			parent := filepath.Dir(rel)
			if parent != "." {
				parentWords := strings.ReplaceAll(parent, string(filepath.Separator), " ")
				if !strings.HasPrefix(name, parentWords+" ") && !strings.HasPrefix(name, parentWords+"-") && name != parentWords {
					name = parentWords + " " + name
				}
			}
		}
		title := "CLI Command: " + name
		desc := strings.TrimSpace(strFromAny(m[objects.FieldKeyShort]))
		def := fmt.Sprintf("CLI command `%s` defined in `%s`.", name, relPath(root, path))
		if desc != emptyValue {
			def += " " + oneLine(desc)
		}
		hint := mustJSON(map[string]any{
			"source_kind":  "command_spec",
			"command_name": name,
			"spec_path":    relPath(root, path),
		})
		out = append(out, glossaryCandidate{
			Title:       title,
			Category:    "cli",
			Context:     "operational",
			Definition:  def,
			AgentPrompt: "Use when explaining or suggesting this CLI command and its purpose.",
			MachineHint: hint,
			SourceType:  "command_spec",
			SourcePath:  relPath(root, path),
		})
		return nil
	})
	return out, err
}

func scanSpecCandidates(root, dir string) ([]glossaryCandidate, error) {
	var files []string
	_ = filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	out := make([]glossaryCandidate, 0, len(files))
	for _, path := range files {
		m, err := readYAMLMap(path)
		if err != nil {
			return nil, errfmt.Errorf("read spec %s: %w", path, err)
		}
		ontology := objectSpecOntologyName(m)
		if ontology == emptyValue {
			continue
		}
		title := "Object kind: " + ontology
		desc := strings.TrimSpace(strFromAny(m[objects.FieldKeyDescription]))
		def := fmt.Sprintf("Spec-defined object kind `%s` from `%s`.", ontology, relPath(root, path))
		if desc != emptyValue {
			def += " " + oneLine(desc)
		}
		hint := mustJSON(map[string]any{
			"source_kind":            "object_spec",
			objects.FieldKeyOntology: ontology,
			"spec_path":              relPath(root, path),
		})
		out = append(out, glossaryCandidate{
			Title:       title,
			Category:    "architecture",
			Context:     "operational",
			Definition:  def,
			AgentPrompt: "Use when mapping high-level intent to this spec-defined object kind and its lifecycle/fields.",
			MachineHint: hint,
			SourceType:  "object_spec",
			SourcePath:  relPath(root, path),
		})
	}
	return out, nil
}

// objectSpecOntologyName returns the kind name from an object_spec YAML map.
// Canonical field is ontology; some stubs only set kind (e.g. qa_success.yaml).
func objectSpecOntologyName(m map[string]any) string {
	if m == nil {
		return emptyValue
	}
	ontology := strings.TrimSpace(strFromAny(m[objects.FieldKeyOntology]))
	if ontology != emptyValue {
		return ontology
	}
	return strings.TrimSpace(strFromAny(m[objects.FieldKeyKind]))
}

func scanLifecycleCandidates(root, dir string) ([]glossaryCandidate, error) {
	var files []string
	_ = filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	out := make([]glossaryCandidate, 0, len(files))
	for _, path := range files {
		m, err := readYAMLMap(path)
		if err != nil {
			return nil, errfmt.Errorf("read lifecycle %s: %w", path, err)
		}
		objType := strings.TrimSpace(strFromAny(m[objects.FieldKeyObjectType]))
		if objType == emptyValue {
			continue
		}
		title := "Lifecycle: " + objType
		hint := mustJSON(map[string]any{
			"source_kind":              "lifecycle",
			objects.FieldKeyObjectType: objType,
			"lifecycle_path":           relPath(root, path),
		})
		out = append(out, glossaryCandidate{
			Title:       title,
			Category:    "process",
			Context:     "operational",
			Definition:  fmt.Sprintf("Lifecycle contract for object type `%s` from `%s`.", objType, relPath(root, path)),
			AgentPrompt: "Use when reasoning about status transitions, terminal states, and lifecycle-valid updates.",
			MachineHint: hint,
			SourceType:  "lifecycle",
			SourcePath:  relPath(root, path),
		})
	}
	return out, nil
}

func scanConfigCandidates(root, dir string) ([]glossaryCandidate, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	out := make([]glossaryCandidate, 0, len(files))
	for _, path := range files {
		base := filepath.Base(path)
		name := strings.TrimSuffix(base, ".yaml")
		titleName := strings.ReplaceAll(name, "_", " ")
		title := "Config surface: " + titleName
		hint := mustJSON(map[string]any{
			"source_kind": "config",
			"config_name": name,
			"config_path": relPath(root, path),
		})
		out = append(out, glossaryCandidate{
			Title:       title,
			Category:    "process",
			Context:     "operational",
			Definition:  fmt.Sprintf("Operator/configuration surface defined by `%s`.", relPath(root, path)),
			AgentPrompt: "Use when discussing configurable behavior rooted in this canonical config file.",
			MachineHint: hint,
			SourceType:  "config",
			SourcePath:  relPath(root, path),
		})
	}
	return out, nil
}

func toAbs(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

func relPath(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return r
}

func readYAMLMap(path string) (map[string]any, error) {
	raw, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func strFromAny(v any) string {
	if v == nil {
		return emptyValue
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		// Avoid fmt "%v" of typed-nil / non-strings becoming the literal "<nil>",
		// which previously created glossary titles like "Object kind: <nil>".
		s := strings.TrimSpace(fmt.Sprintf("%v", t))
		if s == "" || s == "<nil>" {
			return emptyValue
		}
		return s
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
