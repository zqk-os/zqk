package system

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/cliapp"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewUpdateSpecsCmd creates a command to bulk update object specs.
// Command structure and flags are from .zqk/cli/specs/system/update_specs_command.yaml.
func NewUpdateSpecsCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemUpdateSpecsCommandBuilder()
	cli.BindAsyncProgress(cmd, runUpdateSpecs)
	return cmd
}

type specUpdate struct {
	filepath   string
	changes    []string
	content    string
	oldContent string
	specName   string
}

func runUpdateSpecs(cmd *cobra.Command, args []string) error {
	_ = args // args not used - required by cobra.Command.RunE signature
	ctx := cli.GetContext(cmd)
	logger := logging.GetLoggerFromProfile(ctx.Profile)

	// Parse flags
	flags := parseUpdateSpecsFlags(cmd)

	// Get project root
	projectRoot, err := getProjectRootForUpdateSpecs(ctx)
	if err != nil {
		return err
	}

	// Handle field operation (early return)
	if err := handleFieldOperation(cmd, projectRoot, flags); err != nil {
		return err
	}

	// Validate inputs
	if err := validateUpdateSpecsInputs(flags); err != nil {
		return err
	}

	// Get specs directory
	specsDir, err := getSpecsDirectory(projectRoot)
	if err != nil {
		return err
	}

	// Collect files to process
	filesToProcess, err := collectFilesToProcess(specsDir, flags.Files, logger)
	if err != nil {
		return err
	}

	// Process files
	updates := processSpecFiles(filesToProcess, flags.TraitGroups, logger, flags.Verbose)

	// Show preview (POL-CODE-007: same writer as cli.WriteOutput / MCP routing)
	out := cli.CommandOutputWriter(cmd, nil)
	showUpdatePreview(updates, out)
	if len(updates) == 0 {
		return nil
	}

	// Handle dry-run mode
	if flags.DryRun {
		handleDryRunMode(updates, flags.Verbose, out)
		return nil
	}

	// Apply updates
	updated, err := applyUpdates(updates, logger, flags.Verbose, out)
	if err != nil {
		return err
	}
	logging.Fluent(logger).Info("Updated spec files").
		Int("count", updated).
		Log()

	// Handle validation
	if err := handleValidation(flags.Validate, updates, projectRoot, logger, out); err != nil {
		return err
	}

	if updated > 0 {
		idx, err := objects.RefreshMaterializedSpecIndex(projectRoot)
		if err != nil {
			return errfmt.Newf("refresh materialized spec index").Wrap(err)
		}
		logging.Fluent(logger).Info("Refreshed materialized spec index").
			Path(filepath.Join(projectRoot, paths.ProcessInternalDir, "spec_index.json")).
			Int("kind_count", len(idx.Kinds)).
			String("global_spec_cache_revision", strconv.FormatUint(idx.GlobalSpecCacheRevision, 10)).
			String("builder_spec_cache_revision", strconv.FormatUint(idx.BuilderSpecCacheRevision, 10)).
			Log()
		InvalidateDescriptorReadModelCache(projectRoot)
	}

	return nil
}

func showDiff(out io.Writer, oldContent, newContent string) {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	maxLen := len(oldLines)
	if len(newLines) > maxLen {
		maxLen = len(newLines)
	}

	for i := 0; i < maxLen; i++ {
		oldLine := ""
		newLine := ""
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if oldLine != newLine {
			if oldLine != emptyValue {
				_, _ = fmt.Fprintf(out, "- %s\n", oldLine)
			}
			if newLine != emptyValue {
				_, _ = fmt.Fprintf(out, "+ %s\n", newLine)
			}
		}
	}
}

func validateUpdatedSpecs(updates []specUpdate, _ string, logger logging.Logger) error {
	specLoader := objects.GetGlobalSpecLoader()
	specLoader.ClearCache()
	ctx := pkgctx.NewSystemContext()

	var validationErrors []error
	for _, update := range updates {
		specName := filepath.Base(update.filepath)
		spec, errs, err := objects.LoadSpecAndValidate(ctx, specLoader, specName, nil)
		if err != nil {
			validationErrors = append(validationErrors, errfmt.Errorf("failed to load spec %s: %w", specName, err))
			continue
		}
		if spec.Ontology == emptyValue {
			validationErrors = append(validationErrors, errfmt.Errorf("spec %s missing ontology", specName))
			continue
		}
		if spec.Extends == emptyValue && spec.Ontology != objects.KindBaseObject && spec.Ontology != objects.KindAuditable && spec.Ontology != objects.KindWorkInterval && spec.Ontology != objects.KindWorkUnit {
			validationErrors = append(validationErrors, errfmt.Errorf("spec %s missing extends", specName))
			continue
		}

		if len(errs) > 0 {
			first := errs[0]
			validationErrors = append(validationErrors, errfmt.Errorf(
				"spec %s: %d validation issue(s) (e.g. field %q: %s)",
				specName, len(errs), first.Field, first.Message))
		}
	}

	if len(validationErrors) > 0 {
		for _, verr := range validationErrors {
			logging.Fluent(logger).Error("Validation error", verr).Log()
		}
		return errfmt.Errorf("validation failed for %d spec(s)", len(validationErrors))
	}

	return nil
}

func processSpecFile(
	specFilePath string,
	traitGroups bool,
	traitRegistry *objects.TraitRegistry,
	logger logging.Logger,
	_ bool,
) (*specUpdate, error) {
	data, err := fileutil.ReadFile(specFilePath)
	if err != nil {
		return nil, errfmt.Newf("failed to read file").Wrap(err)
	}

	var spec struct {
		Ontology string         `yaml:"ontology"`
		Extends  string         `yaml:"extends"`
		Traits   []string       `yaml:"traits"`
		Fields   map[string]any `yaml:"fields"`
	}

	if err := yaml.Unmarshal(data, &spec); err != nil {
		// Log but don't fail - some specs might have non-standard YAML
		logging.Fluent(logger).Debug("Failed to parse spec as standard structure, skipping").
			File(specFilePath).
			WithError(err).
			Log()
		return nil, nil
	}

	// Skip specs that don't have an ontology (likely invalid or placeholder)
	if spec.Ontology == emptyValue {
		return nil, nil
	}

	var changes []string
	content := string(data)
	oldContent := content

	// Update object-level trait groups
	if traitGroups {
		updated, newContent, objChanges := updateObjectTraitGroups(content, spec.Extends, spec.Traits, traitRegistry)
		if updated {
			content = newContent
			changes = append(changes, objChanges...)
		}

		// Update field-level trait groups
		updated, newContent, fieldChanges := updateFieldTraitGroups(content, spec.Fields, traitRegistry)
		if updated {
			content = newContent
			changes = append(changes, fieldChanges...)
		}
	}

	if len(changes) == 0 {
		return nil, nil
	}

	return &specUpdate{
		filepath:   specFilePath,
		changes:    changes,
		content:    content,
		oldContent: oldContent,
		specName:   spec.Ontology,
	}, nil
}

func updateObjectTraitGroups(content, extends string, traits []string, traitRegistry *objects.TraitRegistry) (bool, string, []string) {
	// Get base traits from registry
	config, err := getBaseTraitsForExtends(extends, traitRegistry)
	if err != nil {
		return false, content, nil
	}

	// Check if traits contain all base traits and extract extras
	hasAllBase, extras := checkTraitsContainBase(config.BaseTraits, traits)
	if !hasAllBase {
		return false, content, nil
	}

	// Replace traits section
	replaced, newContent := replaceObjectTraitsSection(content, config.Replacement, extras)
	if !replaced {
		return false, content, nil
	}

	return true, newContent, []string{fmt.Sprintf("Object traits: replaced with %s", config.Replacement)}
}

func updateFieldTraitGroups(content string, fields map[string]any, traitRegistry *objects.TraitRegistry) (bool, string, []string) {
	if fields == nil {
		return false, content, nil
	}

	fieldPatterns := getFieldTraitGroupPatterns()
	lines := strings.Split(content, "\n")
	var newLines []string
	var changes []string
	state := &FieldTraitState{}

	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := getIndent(line)

		// Detect field start
		if !state.InField {
			if isField, fieldName := detectFieldStart(trimmed, indent, fields); isField {
				state.InField = true
				state.CurrentField = fieldName
				state.FieldIndent = indent
				newLines = append(newLines, line)
				continue
			}
		}

		// Process field content
		if state.InField {
			if detectFieldEnd(trimmed, indent, state.FieldIndent) {
				// New field or end of fields section
				state.InField = false
				state.CurrentField = ""
				state.InFieldTraits = false
			} else if detectFieldTraitsStart(trimmed, indent, state.FieldIndent) {
				// Field traits section
				state.InFieldTraits = true
				newLines = append(newLines, line)
				continue
			} else if state.InFieldTraits {
				// Process trait lines
				if strings.HasPrefix(trimmed, "-") {
					// This is a trait line - we'll collect and replace
					continue
				} else if trimmed != emptyValue && indent <= state.FieldIndent+4 {
					// End of traits section - time to replace
					traitLines, changeMsg := processFieldTraitsEnd(lines, lineIdx, state.FieldIndent, state.CurrentField, fieldPatterns, traitRegistry)
					newLines = append(newLines, traitLines...)
					if changeMsg != emptyValue {
						changes = append(changes, changeMsg)
					}
					state.InFieldTraits = false
				}
			}
		}

		if !state.InFieldTraits {
			newLines = append(newLines, line)
		}
	}

	if len(changes) > 0 {
		return true, strings.Join(newLines, "\n"), changes
	}

	return false, content, nil
}

func extractFieldTraits(lines []string, startIdx, fieldIndent int) []string {
	var traits []string
	for i := startIdx; i >= 0; i-- {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		indent := getIndent(line)

		if strings.HasPrefix(trimmed, "traits:") {
			break
		}
		if strings.HasPrefix(trimmed, "-") && indent > fieldIndent {
			trait := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			traits = append([]string{trait}, traits...) // Prepend to maintain order
		} else if trimmed != emptyValue && indent <= fieldIndent+4 {
			break
		}
	}
	return traits
}

func findFieldTraitGroupReplacement(traits []string, patterns map[string]string, _ *objects.TraitRegistry) string {
	sort.Strings(traits)
	traitsStr := strings.Join(traits, ",")

	for groupName, pattern := range patterns {
		patternTraits := strings.Split(pattern, ",")
		sort.Strings(patternTraits)
		patternStr := strings.Join(patternTraits, ",")

		// Check if traits match pattern exactly
		if traitsStr == patternStr {
			return groupName
		}

		// Check if traits contain all pattern traits (subset match)
		hasAll := true
		for _, pt := range patternTraits {
			if !containsStringInSlice(traits, pt) {
				hasAll = false
				break
			}
		}

		if hasAll && len(traits) == len(patternTraits) {
			return groupName
		}
	}

	return ""
}

func getIndent(line string) int {
	for i, r := range line {
		if r != ' ' && r != '\t' {
			return i
		}
	}
	return len(line)
}

func containsStringInSlice(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
