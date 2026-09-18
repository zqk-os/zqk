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

// NewSpecFieldsCmd creates the "spec fields" subcommand.
func NewSpecFieldsCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSpecFieldsCommandBuilder(), &cobra.Command{
		Use:   "fields <kind>",
		Short: "Inspect field schema and validation rules for an object kind",
		Long:  "List all fields and their types, permissions, validation rules, and purposes for an object kind specification.",
	})

	cli.BindAsyncProgress(cmd, runSpecFields)
	return cmd
}

func runSpecFields(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		kind := strings.TrimSpace(args[0])
		kind = strings.TrimPrefix(kind, "SPEC-")
		kind = strings.ToLower(kind)

		specLoader := objects.NewSpecLoader(proc.ProjectRoot())
		raw, _ := cmd.Flags().GetBool("raw")
		requiredOnly, _ := cmd.Flags().GetBool("required-only")

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

		type fieldItem struct {
			Name         string         `json:"name" yaml:"name"`
			Type         string         `json:"type" yaml:"type"`
			Required     bool           `json:"required" yaml:"required"`
			SemanticType string         `json:"semantic_type,omitempty" yaml:"semantic_type,omitempty"`
			Permissions  string         `json:"permissions,omitempty" yaml:"permissions,omitempty"`
			Purpose      string         `json:"purpose,omitempty" yaml:"purpose,omitempty"`
			Validation   map[string]any `json:"validation,omitempty" yaml:"validation,omitempty"`
		}

		var items []fieldItem
		for fn, fv := range fieldsMap {
			fMap, ok := fv.(map[string]any)
			if !ok {
				continue
			}

			isRequired := false
			if vBlock, ok := fMap["validation"].(map[string]any); ok {
				if req, ok := vBlock["required"].(bool); ok && req {
					isRequired = true
				}
			}

			if requiredOnly && !isRequired {
				continue
			}

			fType, _ := fMap["type"].(string)
			if fType == "" {
				fType = "unknown"
			}
			semType, _ := fMap["semantic_type"].(string)
			perm, _ := fMap["permissions"].(string)

			purpose := ""
			if chk, ok := fMap["checklist"].(map[string]any); ok {
				if p, ok := chk["purpose"].(string); ok {
					purpose = p
				}
			}

			var valMap map[string]any
			if v, ok := fMap["validation"].(map[string]any); ok {
				valMap = v
			}

			items = append(items, fieldItem{
				Name:         fn,
				Type:         fType,
				Required:     isRequired,
				SemanticType: semType,
				Permissions:  perm,
				Purpose:      purpose,
				Validation:   valMap,
			})
		}

		sort.Slice(items, func(i, j int) bool {
			if items[i].Required != items[j].Required {
				return items[i].Required // required first
			}
			return items[i].Name < items[j].Name
		})

		format := cli.GetFormat(cmd)
		if format == "json" || format == "yaml" || format == "yml" {
			data := map[string]any{
				"ontology":    spec.Ontology,
				"total_count": len(items),
				"fields":      items,
			}
			return cli.FormatOutput(cmd, data)
		}

		// Table format
		var buf strings.Builder
		buf.WriteString(fmt.Sprintf("Fields for %s (%d fields)\n", spec.Ontology, len(items)))
		buf.WriteString(strings.Repeat("=", len(spec.Ontology)+25) + "\n\n")

		colName := "FIELD"
		colType := "TYPE"
		colReq := "REQ"
		colSem := "SEMANTIC"
		colPerm := "PERM"
		colPurpose := "PURPOSE"

		maxName := len(colName)
		maxType := len(colType)
		maxSem := len(colSem)
		maxPerm := len(colPerm)

		for _, it := range items {
			if len(it.Name) > maxName {
				maxName = len(it.Name)
			}
			if len(it.Type) > maxType {
				maxType = len(it.Type)
			}
			if len(it.SemanticType) > maxSem {
				maxSem = len(it.SemanticType)
			}
			if len(it.Permissions) > maxPerm {
				maxPerm = len(it.Permissions)
			}
		}

		formatStr := fmt.Sprintf("%%-%ds  %%-%ds  %%-3s  %%-%ds  %%-%ds  %%s\n", maxName, maxType, maxSem, maxPerm)
		buf.WriteString(fmt.Sprintf(formatStr, colName, colType, colReq, colSem, colPerm, colPurpose))
		buf.WriteString(fmt.Sprintf(formatStr,
			strings.Repeat("-", maxName),
			strings.Repeat("-", maxType),
			"---",
			strings.Repeat("-", maxSem),
			strings.Repeat("-", maxPerm),
			strings.Repeat("-", 30),
		))

		for _, it := range items {
			reqStr := "no"
			if it.Required {
				reqStr = "yes"
			}
			pSummary := it.Purpose
			if len(pSummary) > 60 {
				pSummary = pSummary[:57] + "..."
			}
			buf.WriteString(fmt.Sprintf(formatStr, it.Name, it.Type, reqStr, it.SemanticType, it.Permissions, pSummary))
		}

		return cli.WriteOutput(cmd, []byte(buf.String()))
	})(cmd, args)
}
