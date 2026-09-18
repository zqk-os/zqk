package system

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type syncIDPrefixesFromSpecsReport struct {
	DryRun         bool               `json:"dry_run" yaml:"dry_run"`
	SpecsDir       string             `json:"specs_dir" yaml:"specs_dir"`
	ConfigFile     string             `json:"config_file" yaml:"config_file"`
	TotalSpecs     int                `json:"total_specs" yaml:"total_specs"`
	PrefixUpdates  int                `json:"prefix_updates" yaml:"prefix_updates"`
	SynonymUpdates int                `json:"synonym_updates" yaml:"synonym_updates"`
	Details        []syncPrefixDetail `json:"details,omitempty" yaml:"details,omitempty"`
}

type syncPrefixDetail struct {
	Ontology string   `json:"ontology" yaml:"ontology"`
	Type     string   `json:"type" yaml:"type"` // "prefix" or "synonym"
	Action   string   `json:"action" yaml:"action"`
	From     []string `json:"from" yaml:"from"`
	To       []string `json:"to" yaml:"to"`
}

// NewSyncIDPrefixesFromSpecsCmd wires command builder from spec:
// .zqk/cli/specs/system/sync_id_prefixes_from_specs_command.yaml
func NewSyncIDPrefixesFromSpecsCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemSyncIdPrefixesFromSpecsCommandBuilder()
	cmd.Args = cobra.NoArgs
	cmd.RunE = runSyncIDPrefixesFromSpecs
	return cmd
}

func runSyncIDPrefixesFromSpecs(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var err error
		_ = err
		root := proc.ProjectRoot()
		if root == emptyValue {
			return errfmt.Errorf("project root not found")
		}

		specsDir, _ := cmd.Flags().GetString("specs-dir")
		configFile, _ := cmd.Flags().GetString("config-file")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		apply, _ := cmd.Flags().GetBool("apply")

		if specsDir == emptyValue {
			specsDir = paths.ProcessInternalObjectSpecsDir
		}
		if configFile == emptyValue {
			configFile = filepath.Join(root, paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile)
		}

		specsDir = toAbs(root, specsDir)
		configFile = toAbs(root, configFile)

		// Load existing config
		cfg, err := validation.LoadIDPrefixesConfig(configFile)
		if err != nil {
			return errfmt.Newf("failed to load ID prefixes config").Wrap(err)
		}

		// Initialize maps if nil
		if cfg.KindToPrefixes == nil {
			cfg.KindToPrefixes = make(map[string][]string)
		}
		if cfg.KindToSynonyms == nil {
			cfg.KindToSynonyms = make(map[string][]string)
		}

		// Load all specs
		loader := objects.NewSpecLoader(specsDir)
		ontologies, err := loader.DiscoverOntologies()
		if err != nil {
			return errfmt.Newf("failed to discover ontologies").Wrap(err)
		}

		rep := syncIDPrefixesFromSpecsReport{
			DryRun:     dryRun || !apply,
			SpecsDir:   specsDir,
			ConfigFile: configFile,
			TotalSpecs: len(ontologies),
		}

		for _, ontology := range ontologies {
			spec, err := loader.LoadSpec(ontology)
			if err != nil {
				continue
			}

			// Sync ID prefixes
			if len(spec.IDPrefixes) > 0 {
				existing := cfg.KindToPrefixes[ontology]
				if !slices.Equal(existing, spec.IDPrefixes) {
					rep.Details = append(rep.Details, syncPrefixDetail{
						Ontology: ontology,
						Type:     "prefix",
						Action:   "update",
						From:     existing,
						To:       spec.IDPrefixes,
					})
					cfg.KindToPrefixes[ontology] = spec.IDPrefixes
					rep.PrefixUpdates++
				}
			}

			// Sync ID synonyms
			if len(spec.IDSynonyms) > 0 {
				existing := cfg.KindToSynonyms[ontology]
				if !slices.Equal(existing, spec.IDSynonyms) {
					rep.Details = append(rep.Details, syncPrefixDetail{
						Ontology: ontology,
						Type:     "synonym",
						Action:   "update",
						From:     existing,
						To:       spec.IDSynonyms,
					})
					cfg.KindToSynonyms[ontology] = spec.IDSynonyms
					rep.SynonymUpdates++
				}
			}
		}

		if rep.PrefixUpdates == 0 && rep.SynonymUpdates == 0 {
			return outputSyncIDPrefixesReport(cmd, rep)
		}

		if !rep.DryRun {
			// Save config
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return errfmt.Newf("failed to marshal config").Wrap(err)
			}

			// Prepend schema comment
			output := []byte("$schema: \"../../../.zqk/cli/specs/schemas/id_prefixes_config.schema.json\"\n\n" + string(data))

			if err := fileutil.WriteSecureFile(configFile, output); err != nil {
				return errfmt.Newf("failed to write config file").Wrap(err)
			}
		}

		return outputSyncIDPrefixesReport(cmd, rep)
	})(cmd, nil)
}

func outputSyncIDPrefixesReport(cmd *cobra.Command, rep syncIDPrefixesFromSpecsReport) error {
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, rep)
	default:
		var b strings.Builder
		b.WriteString("sync-id-prefixes-from-specs\n")
		b.WriteString("---------------------------\n")
		fmt.Fprintf(&b, "dry_run: %v\n", rep.DryRun)
		fmt.Fprintf(&b, "config_file: %s\n", rep.ConfigFile)
		fmt.Fprintf(&b, "total_specs_scanned: %d\n", rep.TotalSpecs)
		fmt.Fprintf(&b, "prefix_updates: %d\n", rep.PrefixUpdates)
		fmt.Fprintf(&b, "synonym_updates: %d\n", rep.SynonymUpdates)

		if len(rep.Details) > 0 {
			b.WriteString("\nChanges:\n")
			for _, d := range rep.Details {
				fmt.Fprintf(&b, "  - %s (%s): %v -> %v\n", d.Ontology, d.Type, d.From, d.To)
			}
			if rep.DryRun {
				b.WriteString("\nApply:\n")
				b.WriteString("  zqk system sync-id-prefixes-from-specs --apply --dry-run=false\n")
			}
		} else {
			b.WriteString("\nNo changes detected.\n")
		}
		return cli.WriteOutput(cmd, []byte(b.String()))
	}
}
