package test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testdiscovery"
	"gopkg.in/yaml.v3"
)

// NewDiscoverCmd creates the `zqk test discover` command.
func NewDiscoverCmd() *cobra.Command {
	cmd := bldr.NewTestDiscoverCommandBuilder()

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			clipkg.ResetTimeout()

			langFilter, _ := cmd.Flags().GetString("lang")
			syncToKernel, _ := cmd.Flags().GetBool("sync")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			incremental, _ := cmd.Flags().GetBool("incremental")
			workers, _ := cmd.Flags().GetInt("workers")
			verbose, _ := cmd.Flags().GetBool("verbose")
			format, _ := cmd.Flags().GetString("format")

			var languages []string
			if langFilter != "" {
				for _, l := range strings.Split(langFilter, ",") {
					l = strings.TrimSpace(l)
					if l != "" {
						languages = append(languages, l)
					}
				}
			}

			paths := args
			if len(paths) == 0 {
				paths = []string{"."}
			} else {
				for i, p := range paths {
					if filepath.IsAbs(p) {
						rel, err := filepath.Rel(proc.ProjectRoot(), p)
						if err == nil && !strings.HasPrefix(rel, "..") {
							paths[i] = rel
						}
					}
				}
			}

			opts := testdiscovery.DiscoveryOptions{
				ProjectRoot: proc.ProjectRoot(),
				Paths:       paths,
				Languages:   languages,
				Workers:     workers,
				Incremental: incremental,
			}

			start := time.Now()
			engine := testdiscovery.NewEngine()
			ctx := cmd.Context()

			targets, err := engine.Discover(ctx, opts)
			if err != nil {
				return errfmt.Newf("test discovery failed").Wrap(err)
			}
			duration := time.Since(start)

			out := cmd.OutOrStdout()

			if strings.EqualFold(format, "json") {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(targets)
			} else if strings.EqualFold(format, "yaml") {
				enc := yaml.NewEncoder(out)
				return enc.Encode(targets)
			}

			fmt.Fprintf(out, "🔍 Discovered %d test target(s) across %s in %v\n\n",
				len(targets), opts.ProjectRoot, duration.Round(time.Millisecond))

			// Group by language
			byLang := make(map[string][]testdiscovery.DiscoveredTarget)
			for _, t := range targets {
				byLang[t.Language] = append(byLang[t.Language], t)
			}

			for lang, list := range byLang {
				fmt.Fprintf(out, "── %s (%d tests) ──\n", strings.ToUpper(lang), len(list))
				for _, t := range list {
					fmt.Fprintf(out, "  • %s:%d  [%s]\n", t.Path, t.Line, t.Function)
					if verbose {
						if len(t.CriteriaRefs) > 0 {
							fmt.Fprintf(out, "      Criteria: %s\n", strings.Join(t.CriteriaRefs, ", "))
						}
						if len(t.Tags) > 0 {
							fmt.Fprintf(out, "      Tags: %s\n", strings.Join(t.Tags, ", "))
						}
						if t.ExecutionCommand != "" {
							fmt.Fprintf(out, "      Cmd: %s\n", t.ExecutionCommand)
						}
					}
				}
				fmt.Fprintln(out)
			}

			if syncToKernel && !dryRun {
				fmt.Fprintf(out, "⚡ Synchronizing %d test_case object(s) to Knowledge Kernel CAS...\n", len(targets))
				sp := proc.Storage()
				secCtx := proc.SecurityContext()

				createdCount := 0
				for _, target := range targets {
					tcObj, err := engine.GenerateTestCaseObject(target, proc.ProjectRoot())
					if err != nil {
						continue
					}
					id := tcObj[objects.FieldKeyID].(string)
					exists, _ := sp.Exists(ctx, secCtx, id)
					if !exists {
						err := sp.Create(ctx, secCtx, tcObj)
						if err == nil {
							createdCount++
						}
					}
				}
				fmt.Fprintf(out, "✓ Successfully synchronized %d new test_case object(s) to CAS.\n", createdCount)
			}

			return nil
		})(cmd, args)
	}

	return cmd
}
