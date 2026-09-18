package spec

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewSpecGetCmd creates the "spec get" / "spec show" subcommand.
func NewSpecGetCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSpecGetCommandBuilder(), &cobra.Command{
		Use:     "get <kind>",
		Aliases: []string{"show"},
		Short:   "Get object specification details",
		Long:    "Display specification details for an object kind including ontology, traits, and field requirements.",
	})

	cli.BindAsyncProgress(cmd, runSpecGet)
	return cmd
}

func runSpecGet(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		kind := strings.TrimSpace(args[0])
		kind = strings.TrimPrefix(kind, "SPEC-")
		kind = strings.ToLower(kind)

		specLoader := objects.NewSpecLoader(proc.ProjectRoot())
		raw, _ := cmd.Flags().GetBool("raw")

		spec, err := specLoader.LoadSpec(kind)
		if err != nil {
			return cli.Guard(cmd).Err(err).Wrapf("failed to load spec: %w").Return()
		}
		if spec == nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("spec not found for kind: %s", kind)).Return()
		}

		fieldsMap := spec.ResolvedFields
		if raw || len(fieldsMap) == 0 {
			fieldsMap = spec.Fields
		}

		fieldNames := make([]string, 0, len(fieldsMap))
		var requiredFields []string
		for fn, fv := range fieldsMap {
			fieldNames = append(fieldNames, fn)
			if fMap, ok := fv.(map[string]any); ok {
				if vBlock, ok := fMap["validation"].(map[string]any); ok {
					if req, ok := vBlock["required"].(bool); ok && req {
						requiredFields = append(requiredFields, fn)
					}
				}
			}
		}
		sort.Strings(fieldNames)
		sort.Strings(requiredFields)

		traits := spec.ResolvedTraits
		if raw || len(traits) == 0 {
			traits = spec.Traits
		}

		format := cli.GetFormat(cmd)
		if format == "json" || format == "yaml" || format == "yml" {
			data := map[string]any{
				"schema_version":  spec.SchemaVersion,
				"ontology":        spec.Ontology,
				"extends":         spec.Extends,
				"composes":        spec.Composes,
				"visibility":      spec.Visibility,
				"namespace":       spec.Namespace,
				"description":     spec.Description,
				"id_prefixes":     spec.IDPrefixes,
				"id_synonyms":     spec.IDSynonyms,
				"storage_profile": spec.StorageProfile,
				"traits":          traits,
				"required_fields": requiredFields,
				"field_count":     len(fieldNames),
				"fields":          fieldsMap,
			}
			return cli.FormatOutput(cmd, data)
		}

		// Table format
		var buf strings.Builder
		buf.WriteString(fmt.Sprintf("Specification: %s\n", spec.Ontology))
		buf.WriteString(strings.Repeat("=", len(spec.Ontology)+15) + "\n\n")
		buf.WriteString(fmt.Sprintf("  Schema Version:    %s\n", spec.SchemaVersion))
		buf.WriteString(fmt.Sprintf("  Ontology:          %s\n", spec.Ontology))
		if spec.Extends != "" {
			buf.WriteString(fmt.Sprintf("  Extends:           %s\n", spec.Extends))
		}
		if len(spec.Composes) > 0 {
			buf.WriteString(fmt.Sprintf("  Composes:          %s\n", strings.Join(spec.Composes, ", ")))
		}
		if spec.Visibility != "" {
			buf.WriteString(fmt.Sprintf("  Visibility:        %s\n", spec.Visibility))
		}
		if spec.StorageProfile != "" {
			buf.WriteString(fmt.Sprintf("  Storage Profile:   %s\n", spec.StorageProfile))
		}
		if len(spec.IDPrefixes) > 0 {
			buf.WriteString(fmt.Sprintf("  ID Prefixes:       %s\n", strings.Join(spec.IDPrefixes, ", ")))
		}
		if spec.Description != "" {
			buf.WriteString(fmt.Sprintf("\nDescription:\n  %s\n", spec.Description))
		}
		if len(traits) > 0 {
			buf.WriteString(fmt.Sprintf("\nTraits (%d):\n", len(traits)))
			for _, t := range traits {
				buf.WriteString(fmt.Sprintf("  - %s\n", t))
			}
		}
		buf.WriteString(fmt.Sprintf("\nFields (%d):\n", len(fieldNames)))
		buf.WriteString(fmt.Sprintf("  Required: %s\n", strings.Join(requiredFields, ", ")))
		buf.WriteString(fmt.Sprintf("  All:      %s\n", strings.Join(fieldNames, ", ")))
		buf.WriteString(fmt.Sprintf("\nTip: Run 'zqk spec fields %s' for field-level attributes and validation rules.\n", spec.Ontology))

		return cli.WriteOutput(cmd, []byte(buf.String()))
	})(cmd, args)
}
