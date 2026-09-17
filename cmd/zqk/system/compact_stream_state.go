package system

import (
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewCompactStreamStateCmd creates the compact-stream-state command.
// Command structure from spec: .zqk/cli/specs/system/compact_stream_state_command.yaml
// (builder: bldr_cli_cmd_v1.NewSystemCompactStreamStateCommandBuilder).
func NewCompactStreamStateCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemCompactStreamStateCommandBuilder(), &cobra.Command{Use: "compact-stream-state"})
	cli.EnsureCmdAnnotations(cmd)
	cmd.Annotations[cli.AnnotationKeySystemKindValidate] = cli.KindValidateSystemCompactStream
	cmd.RunE = runCompactStreamState
	return cmd
}

func runCompactStreamState(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	kind, _ := cmd.Flags().GetString("kind")
	kind = strings.TrimSpace(kind)
	all, _ := cmd.Flags().GetBool("all")

	if kind == emptyValue && !all {
		return errfmt.Errorf("--kind <kind> or --all is required")
	}
	if kind != emptyValue && all {
		return errfmt.Errorf("--kind and --all are mutually exclusive")
	}

	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	if kind != emptyValue && !all {
		if k, ok := cli.KindCanonicalFromPRERun(cli.KindAnnotKeysSystem, cmd); ok {
			kind = k
		} else {
			var err error
			kind, err = objects.ResolveAndValidateKindForProject(projectRoot, kind)
			if err != nil {
				return err
			}
		}
	}

	logger := logging.GetLoggerFromProfile(ctx.Profile)

	var kinds []string
	if all {
		kinds = storagepkg.StreamStorageEnabledKindsList()
	} else {
		kinds = []string{kind}
	}

	type kindResult struct {
		Kind         string `json:"kind"`
		RegistryOK   bool   `json:"registry_compacted"`
		FilesRemoved int    `json:"segment_files_removed"`
		BytesFreed   int64  `json:"bytes_freed,omitempty"`
		GCErrors     int    `json:"gc_errors,omitempty"`
		Error        string `json:"error,omitempty"`
	}
	results := make([]kindResult, 0, len(kinds))
	var firstErr error

	for _, k := range kinds {
		r := kindResult{Kind: k}

		if err := storagepkg.CompactStreamRegistryForKind(projectRoot, k); err != nil {
			r.Error = err.Error()
			if firstErr == nil {
				firstErr = errfmt.Newf("compact stream registry for %q", k).Wrap(err)
			}
			results = append(results, r)
			continue
		}
		r.RegistryOK = true

		gc := storagepkg.GCOrphanedStreamSegmentsForKind(projectRoot, k)
		r.FilesRemoved = gc.FilesRemoved
		r.BytesFreed = gc.BytesFreed
		r.GCErrors = gc.Errors

		if gc.FilesRemoved > 0 || gc.Errors > 0 {
			logging.Fluent(logger).Info("Stream segment GC").
				Kind(k).
				Int("files_removed", gc.FilesRemoved).
				Int("bytes_freed", int(gc.BytesFreed)).
				Int("errors", gc.Errors).
				Log()
		}
		results = append(results, r)
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		output := map[string]any{
			"project_root": projectRoot,
			"results":      results,
		}
		return cli.FormatOutput(cmd, output)
	}

	for _, r := range results {
		if r.Error != emptyValue {
			logging.Fluent(logger).Warn("Stream state compaction failed").
				Kind(r.Kind).
				String("error", r.Error).
				Log()
			continue
		}
		logging.Fluent(logger).Info("Stream state compaction completed").
			Kind(r.Kind).
			Int("segment_files_removed", r.FilesRemoved).
			Int("bytes_freed", int(r.BytesFreed)).
			Log()
	}
	return firstErr
}
