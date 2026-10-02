package system

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck/snapshot"
)

// NewStateDiffCmd creates the state-diff command
func NewStateDiffCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Compare two system state snapshots or a snapshot against live state",
		"Calculate and display the differences between two .csnap files, or between one .csnap file and the current live system state.",
		"",
		"This tool helps visualize administrative velocity and object mutation across time boundaries.",
	).
		AddExample("Diff two csnap files", "%s system state-diff before.csnap after.csnap").
		AddExample("Diff a csnap against current live system state", "%s system state-diff .zqk-state/system-state.csnap").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStateDiffCommandBuilder(), &cobra.Command{
		Use:  "state-diff <snapshot-a> [snapshot-b]",
		Args: cobra.RangeArgs(1, 2),
		RunE: runStateDiff,
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)

	cmd.Flags().Bool("brief", false, "Only show counts, do not show detailed field changes")

	return cmd
}

func runStateDiff(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ctx, secCtx, projectRoot := processorContexts(proc)

		brief, _ := cmd.Flags().GetBool("brief")

		pathA := args[0]
		if !filepath.IsAbs(pathA) {
			pathA = filepath.Join(projectRoot, pathA)
		}

		cmd.Printf("Loading snapshot A: %s\n", pathA)
		objsA, err := snapshot.ReadAndExpandSnapshot(pathA)
		if err != nil {
			return errfmt.Newf("failed to read snapshot A").Wrap(err)
		}

		var objsB []map[string]any

		if len(args) == 2 {
			pathB := args[1]
			if !filepath.IsAbs(pathB) {
				pathB = filepath.Join(projectRoot, pathB)
			}
			cmd.Printf("Loading snapshot B: %s\n", pathB)
			objsB, err = snapshot.ReadAndExpandSnapshot(pathB)
			if err != nil {
				return errfmt.Newf("failed to read snapshot B").Wrap(err)
			}
		} else {
			cmd.Printf("Extracting live system state for comparison...\n")
			sf, err := storage.NewStorageFactory(ctx, projectRoot)
			if err != nil {
				return errfmt.Newf("failed to initialize storage").Wrap(err)
			}
			storageProvider := sf.GetStorage()
			if storageProvider == nil {
				return errfmt.Errorf("failed to get storage provider")
			}
			storageCtx := &pkgctx.StorageContext{}

			// We can't just pass an empty filter if the storage driver requires Kind.
			// The state_commit command loops through all known kinds. We should do the same.
			kinds := []string{
				"domain_registry", "evolution_management", "brand", "policy", "account", "role",
				"team", "partnership", "goal", "priority_plan", "question", "milestone",
				"criteria", "decision",
				"requirement", "roadmap", "test_case", "backlog_item", "doc_entry",
				"convergence_session", "component", "workstream", "improvement_report",
				"metrics_feedback", "vision", "mission", "strategic_plan", "synonym",
				"namespace", "scenario", "import_tracking", "organization", "division",
				"department", "organizational_change", "impact_analysis", "namespace_registry",
				"metadata_package", "certificate", "keystore_entry", "glossary_term",
				"glossary_term_relation", "library", "display", "persona", "release", "resolver",
				"rollback_report", "rule", "agent_architecture", "agent_feed",
				"agent_onboarding_preparation", "auth_strategy", "bucketing_strategy",
				"context_refresh_schedule", "sampler_profile", "technical_debt",
				"test_command_rule", "workstream_transition", "verification_matrix",
				"corporate_initiative", "important_date", "prompt_template",
				"stakeholder_profile", "strategic_context", "workflow", "vocabulary_scheme",
				"agent_skill", "risk_blocker", "agent_task",
			}
			for _, kind := range kinds {
				filter := storage.ListFilter{Kind: kind}
				result, err := storageProvider.List(context.Background(), secCtx, storageCtx, filter) // Background: request-or-shutdown derived
				if err == nil && len(result.Objects) > 0 {
					objsB = append(objsB, result.Objects...)
				}
			}
		}

		cmd.Printf("\n=== Diff Results ===\n")

		mapA := make(map[string]map[string]any)
		for _, obj := range objsA {
			if id, ok := obj[objects.FieldKeyID].(string); ok {
				mapA[id] = obj
			}
		}

		mapB := make(map[string]map[string]any)
		for _, obj := range objsB {
			if id, ok := obj[objects.FieldKeyID].(string); ok {
				mapB[id] = obj
			}
		}

		var added, removed, modified []string

		for id, objB := range mapB {
			objA, exists := mapA[id]
			if !exists {
				kind, _ := objB[objects.FieldKeyKind].(string)
				added = append(added, fmt.Sprintf("+ %s (%s)", id, kind))
			} else {
				match, diffs := compareObjects(objA, objB, nil)
				if !match {
					kind, _ := objB[objects.FieldKeyKind].(string)
					modStr := fmt.Sprintf("~ %s (%s)", id, kind)
					if !brief {
						// sort diffs for stable output
						var diffKeys []string
						for k := range diffs {
							diffKeys = append(diffKeys, k)
						}
						sort.Strings(diffKeys)
						for _, k := range diffKeys {
							modStr += fmt.Sprintf("\n    %s", diffs[k])
						}
					}
					modified = append(modified, modStr)
				}
			}
		}

		for id, objA := range mapA {
			if _, exists := mapB[id]; !exists {
				kind, _ := objA[objects.FieldKeyKind].(string)
				removed = append(removed, fmt.Sprintf("- %s (%s)", id, kind))
			}
		}

		sort.Strings(added)
		sort.Strings(removed)
		sort.Strings(modified)

		cmd.Printf("Objects in A: %d\n", len(objsA))
		cmd.Printf("Objects in B: %d\n", len(objsB))
		cmd.Printf("\nSummary: %d added, %d removed, %d modified\n", len(added), len(removed), len(modified))

		if len(added) > 0 && !brief {
			cmd.Printf("\n[Added Objects]\n")
			for _, s := range added {
				cmd.Println(s)
			}
		}

		if len(removed) > 0 && !brief {
			cmd.Printf("\n[Removed Objects]\n")
			for _, s := range removed {
				cmd.Println(s)
			}
		}

		if len(modified) > 0 {
			cmd.Printf("\n[Modified Objects]\n")
			for _, s := range modified {
				cmd.Println(s)
			}
		}

		return nil
	})(cmd, args)
}
