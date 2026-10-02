package object

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
)

// getCommandOutputWriter returns the writer for command output (stdout or test buffer).
func getCommandOutputWriter(cmd *cobra.Command) interface{ Write([]byte) (int, error) } {
	ctx := cmd.Context()
	if ctx == nil {
		return nil
	}
	return logging.GetCommandOutputWriter(ctx)
}

// NewTemplateCmd creates a new template command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewTemplateCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectTemplateCommandBuilder()
	// Community draft canon: prefer this over quick/new.
	if cmd.Long != "" {
		cmd.Long = "Canonical draft path: generate YAML, edit, then object create --file.\n" + paths.RewriteCanonicalCLIInvocations("(Alternatives: zqk quick … for text/markdown; zqk new … for scenario drafts.)\n\n") + cmd.Long
	}
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runTemplate)
	configureKindPositionalValidation(cmd, true)
	return cmd
}

func runTemplate(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		kind, err := resolvePositional0Kind(cmd, proc, args, "")
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		logging.FluentEvent(proc.Logger()).Debug("Generating template").
			Kind(kind).
			Log()

		// Load field registry to get field information
		// Reload to ensure we pick up newly added specs (like workflow)
		fieldRegistry := objects.GetGlobalFieldRegistry()
		if err := fieldRegistry.Reload(); err != nil {
			return cli.EnhanceError(cmd, errfmt.Newf("failed to load field registry").Wrap(err))
		}

		kindFields, err := fieldRegistry.GetFieldsForKind(kind)
		if err != nil {
			return cli.EnhanceError(cmd, errfmt.Errorf("failed to get fields for kind %s: %w", kind, err))
		}

		// Generate template
		template := generateTemplate(kind, kindFields, proc.Logger())

		// Get output flag (from common flags)
		//nolint:errcheck // Flag get - error indicates flag not set, default used
		outputPath, _ := cmd.Flags().GetString(cli.FlagOutput)

		// Write template ("-" / empty → stdout; POSIX convention for CLI --output)
		if outputPath != emptyValue && outputPath != "-" {
			if err := fileutil.WriteFile(outputPath, []byte(template), paths.FilePerm644); err != nil { //nolint:gosec // Template files - 0600 is acceptable for user-readable templates
				logging.FluentEvent(proc.Logger()).Error("Failed to write template file", err).
					Path(outputPath).
					Log()
				return cli.Guard(cmd).Err(err).Wrapf("failed to write template file: %w").Return()
			}
			logging.FluentEvent(proc.Logger()).Info("Template written").
				Path(outputPath).
				Log()
			// Write status message to stdout only (do not use WriteOutput; it would overwrite the file with the message)
			msg := fmt.Sprintf("Template written to: %s\n", outputPath)
			if w := getCommandOutputWriter(cmd); w != nil {
				_, _ = w.Write([]byte(msg))
			}
			return nil
		}
		return cli.WriteOutput(cmd, []byte(template))
	})(cmd, args)
}

// generateTemplate creates a YAML template for an object kind
func generateTemplate(kind string, kindFields *objects.KindFields, _ *logging.EventLogger) string {
	var builder strings.Builder

	// Write header comment
	fmt.Fprintf(&builder, "# Template for %s object\n", kind)
	fmt.Fprintf(&builder, "# Fill in the values below and use: %s object create %s --file <this-file>\n", paths.CLICommandName, kind)
	builder.WriteString("# (File will be automatically removed after successful creation)\n\n")

	// Start YAML object
	builder.WriteString("kind: ")
	builder.WriteString(kind)
	builder.WriteString("\n")

	// Add schema version
	_, _ = fmt.Fprintf(&builder, "schema_version: %q\n\n", objectSchemaV2)

	// Load lifecycle to get origin status and all status values
	var originStatus string
	var allStatusValues []string
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
	when.When(func() bool { return err == nil }).Then(func() {
		if origin, initErr := lifecycleLoader.GetOriginStatus(kind); initErr == nil {
			originStatus = origin
		}
		for _, status := range lifecycle.Statuses {
			allStatusValues = append(allStatusValues, status.Value)
		}
	}).OrElse(func() {
		originStatus = "proposed"
		allStatusValues = []string{"proposed", "approved", objectStatusInProgress, "implemented", objectStatusArchived}
	}).Run()

	// Add required fields first
	requiredFields := []objects.FieldInfo{}
	optionalFields := []objects.FieldInfo{}

	for i := range kindFields.AllFields {
		field := &kindFields.AllFields[i]
		// Skip kind and schema_version (already added)
		if field.Name == "kind" || field.Name == "schema_version" {
			continue
		}

		when.When(func() bool { return field.Required }).Then(func() {
			requiredFields = append(requiredFields, *field)
		}).OrElse(func() {
			optionalFields = append(optionalFields, *field)
		}).Run()
	}

	// Write required fields
	if len(requiredFields) > 0 {
		builder.WriteString("# Required fields\n")
		for i := range requiredFields {
			field := requiredFields[i]
			writeFieldTemplate(&builder, &field, true, originStatus, allStatusValues)
		}
		builder.WriteString("\n")
	}

	// Write optional fields with comments
	if len(optionalFields) > 0 {
		builder.WriteString("# Optional fields (uncomment and fill in as needed)\n")
		for i := range optionalFields {
			field := optionalFields[i]
			writeFieldTemplate(&builder, &field, false, originStatus, allStatusValues)
		}
	}

	return builder.String()
}

// writeFieldTemplate writes a field to the template
func writeFieldTemplate(builder *strings.Builder, field *objects.FieldInfo, required bool, originStatus string, allStatusValues []string) {
	// Add field comment with description
	if field.Description != emptyValue {
		writeCommentBlock(builder, field.Description)
	}
	if field.Type != emptyValue {
		builder.WriteString("# (type: ")
		builder.WriteString(field.Type)
		builder.WriteString(")")
		if !required {
			builder.WriteString(" [optional]")
		}
		builder.WriteString("\n")
	} else if field.Description != emptyValue && !required {
		builder.WriteString("# [optional]\n")
	}

	// Special handling for status field - use lifecycle values
	when.When(func() bool { return field.Name == "status" && len(allStatusValues) > 0 }).Then(func() {
		builder.WriteString("# Valid values: ")
		for i, statusVal := range allStatusValues {
			if i > 0 {
				builder.WriteString(", ")
			}
			_, _ = fmt.Fprintf(builder, "%q", statusVal)
		}
		builder.WriteString("\n")
	}).OrElseWhen(func() bool { return field.Type == "enum" && len(field.EnumValues) > 0 }).Then(func() {
		builder.WriteString("# Valid values: ")
		for i, enumVal := range field.EnumValues {
			if i > 0 {
				builder.WriteString(", ")
			}
			_, _ = fmt.Fprintf(builder, "%q", enumVal)
		}
		builder.WriteString("\n")
	}).Run()

	// Write field name
	builder.WriteString(field.Name)
	builder.WriteString(": ")

	// Write placeholder value based on type
	placeholder := getPlaceholderValue(field, originStatus)
	when.When(func() bool { return required }).Then(func() {
		builder.WriteString(placeholder)
	}).OrElse(func() {
		builder.WriteString("# ")
		builder.WriteString(placeholder)
	}).Run()
	builder.WriteString("\n\n")
}

func writeCommentBlock(builder *strings.Builder, text string) {
	for line := range strings.SplitSeq(text, "\n") {
		builder.WriteString("#")
		if line != emptyValue {
			builder.WriteString(" ")
			builder.WriteString(line)
		}
		builder.WriteString("\n")
	}
}

// getPlaceholderValue returns a placeholder value for a field based on its type
func getPlaceholderValue(field *objects.FieldInfo, originStatus string) string {
	// Special handling for status field - use origin status from lifecycle
	if field.Name == "status" && originStatus != emptyValue {
		return fmt.Sprintf("%q", originStatus)
	}
	// Base-object required metadata defaults should be valid out of the box so
	// first-run template->create tutorials are executable without hunting format rules.
	switch field.Name {
	case objects.FieldKeyCreatedAt, objects.FieldKeyUpdatedAt:
		return "\"2026-01-01T00:00:00Z\""
	case objects.FieldKeyCreatedBy, objects.FieldKeyUpdatedBy:
		return fmt.Sprintf("%q", pkgctx.SystemAccountID)
	}

	switch field.Type {
	case "string", "text":
		return "\"\""
	case "integer", "number":
		return "0"
	case "boolean":
		return "false"
	case "list":
		return "[]"
	case "map", "object":
		return "{}"
	case "enum":
		// For enum, use first value as placeholder if available
		if len(field.EnumValues) > 0 {
			return fmt.Sprintf("%q", field.EnumValues[0])
		}
		return "\"\""
	case "reference":
		// For references, use the semantic type hint
		if field.SemanticType != emptyValue {
			return fmt.Sprintf("\"# %s reference (e.g., %s:ID-###)\"", field.SemanticType, field.SemanticType)
		}
		return "\"\""
	default:
		return "\"\""
	}
}
