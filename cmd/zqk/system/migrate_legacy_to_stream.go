// migrate_legacy_to_stream.go: migrate legacy YAML objects in .zqk/process/<dir>
// into stream-backed storage so they are not abandoned. Run for each stream-backed
// kind that still has legacy files; optionally remove legacy files after migration.
package system

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/appledouble"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewMigrateLegacyToStreamCmd creates the migrate-legacy-to-stream command.
// Command structure and flags are from .zqk/cli/specs/system/migrate_legacy_to_stream_command.yaml.
func NewMigrateLegacyToStreamCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemMigrateCommandBuilder(), &cobra.Command{Use: "migrate"})
	cli.EnsureCmdAnnotations(cmd)
	cmd.Annotations[cli.AnnotationKeySystemKindValidate] = cli.KindValidateFlagKind
	_ = cmd.MarkFlagRequired("kind")
	cmd.RunE = runMigrateLegacyToStream
	return cmd
}

type migrateLegacyToStreamResult struct {
	Kind            string   `json:"kind" yaml:"kind"`
	DryRun          bool     `json:"dry_run" yaml:"dry_run"`
	Migrated        int      `json:"migrated" yaml:"migrated"`
	AlreadyInStream int      `json:"already_in_stream" yaml:"already_in_stream"`
	Errors          int      `json:"errors" yaml:"errors"`
	FilesRemoved    int      `json:"files_removed,omitempty" yaml:"files_removed,omitempty"`
	ErrorMessages   []string `json:"error_messages,omitempty" yaml:"error_messages,omitempty"`
}

func runMigrateLegacyToStream(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		kind, _ := cmd.Flags().GetString("kind")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		removeLegacy, _ := cmd.Flags().GetBool("remove-legacy")

		projectRoot := ""
		if cliCtx := cli.GetContext(cmd); cliCtx != nil {
			projectRoot = cliCtx.ProjectRoot
		}
		projectRoot = ProjectRootOrResolve(projectRoot)
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found; run from repo or set --project-root")
		}
		if abs, err := filepath.Abs(projectRoot); err == nil {
			projectRoot = abs
		}

		kind = strings.TrimSpace(kind)
		if k, ok := cli.KindCanonicalFromPRERun(cli.KindAnnotKeysSystem, cmd); ok {
			kind = k
		} else {
			var err error
			kind, err = objects.ResolveAndValidateKindForProject(projectRoot, kind)
			if err != nil {
				return err
			}
		}

		if !storage.StreamStorageEnabledForKind(kind) {
			return errfmt.Errorf("kind %q is not stream-backed; migration only applies to stream-backed kinds", kind)
		}
		dirName := objects.GetDirectoryFromKind(kind)
		if dirName == emptyValue {
			return errfmt.Errorf("unknown kind: %s", kind)
		}

		profile := systemProfileHuman
		if c := cli.GetContext(cmd); c != nil {
			profile = c.Profile
		}
		logger := logging.GetLoggerFromProfile(profile)

		kindDir := datacell.CellCASPrimaryDir(projectRoot, dirName)
		if _, err := fileutil.Stat(kindDir); err != nil {
			if fileutil.IsNotExist(err) {
				logging.Fluent(logger).Info("Kind directory does not exist; nothing to migrate").
					Kind(kind).
					String("dir", kindDir).
					Log()
				return outputMigrateLegacyResult(cmd, migrateLegacyToStreamResult{Kind: kind, DryRun: dryRun})
			}
			return errfmt.Newf("kind dir").Wrap(err)
		}

		storage.BuildPathAliasCacheForProject(projectRoot)
		var err error
		_ = err
		provider := proc.Storage()
		if provider == nil {
			return errfmt.Errorf("storage provider is nil")
		}
		secCtx := proc.SecurityContext()
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}
		baseCtx := pkgctx.NewSystemContext()
		ctx := storage.WithSyncCreateForKind(baseCtx, kind)

		var migrated, alreadyInStream, errCount, filesRemoved int
		var errorMessages []string

		err = filepath.WalkDir(kindDir, func(path string, d fileutil.DirEntry, walkErr error) error {
			if walkErr != nil {
				errorMessages = append(errorMessages, path+": "+walkErr.Error())
				errCount++
				return nil //nolint:nilerr // error recorded in errorMessages and errCount
			}
			if d.IsDir() {
				return nil
			}
			if appledouble.SkipPathInTreeWalk(path) {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".yaml") {
				return nil
			}
			data, err := fileutil.ReadFile(path)
			if err != nil {
				errorMessages = append(errorMessages, path+": "+err.Error())
				errCount++
				return nil //nolint:nilerr // error recorded in errorMessages and errCount
			}
			var obj map[string]any
			if err := yaml.Unmarshal(data, &obj); err != nil {
				errorMessages = append(errorMessages, path+": unmarshal: "+err.Error())
				errCount++
				return nil //nolint:nilerr // error recorded in errorMessages and errCount
			}
			idVal := obj[objects.FieldKeyID]
			id, _ := idVal.(string)
			if id == emptyValue {
				errorMessages = append(errorMessages, path+": missing or invalid id")
				errCount++
				return nil
			}
			obj[objects.FieldKeyKind] = kind

			if dryRun {
				migrated++
				logging.Fluent(logger).Info("Would migrate").
					ObjectID(id).
					Path(path).
					Log()
				return nil
			}

			createErr := provider.Create(ctx, secCtx, obj)
			if createErr != nil {
				if errors.Is(createErr, storage.ErrObjectExists) || strings.Contains(createErr.Error(), "already exists") {
					alreadyInStream++
					if removeLegacy {
						if rmErr := fileutil.Remove(path); rmErr == nil {
							filesRemoved++
						}
					}
					return nil
				}
				errorMessages = append(errorMessages, id+": "+createErr.Error())
				errCount++
				return nil
			}
			migrated++
			if removeLegacy {
				if rmErr := fileutil.Remove(path); rmErr == nil {
					filesRemoved++
				}
			}
			return nil
		})
		if err != nil {
			return errfmt.Newf("walk").Wrap(err)
		}

		result := migrateLegacyToStreamResult{
			Kind:            kind,
			DryRun:          dryRun,
			Migrated:        migrated,
			AlreadyInStream: alreadyInStream,
			Errors:          errCount,
			FilesRemoved:    filesRemoved,
			ErrorMessages:   errorMessages,
		}
		logging.Fluent(logger).Info("Migrate legacy to stream complete").
			Kind(kind).
			Bool("dry_run", dryRun).
			Int("migrated", migrated).
			Int("already_in_stream", alreadyInStream).
			Int("errors", errCount).
			Int("files_removed", filesRemoved).
			Log()
		return outputMigrateLegacyResult(cmd, result)
	})(cmd, nil)
}

func outputMigrateLegacyResult(cmd *cobra.Command, r migrateLegacyToStreamResult) error {
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, r)
	default:
		var buf strings.Builder
		fmt.Fprintf(&buf, "kind: %s\n", r.Kind)
		fmt.Fprintf(&buf, "dry_run: %v\n", r.DryRun)
		fmt.Fprintf(&buf, "migrated: %d\n", r.Migrated)
		fmt.Fprintf(&buf, "already_in_stream: %d\n", r.AlreadyInStream)
		fmt.Fprintf(&buf, "errors: %d\n", r.Errors)
		if r.FilesRemoved > 0 {
			fmt.Fprintf(&buf, "files_removed: %d\n", r.FilesRemoved)
		}
		for _, msg := range r.ErrorMessages {
			buf.WriteString("  - " + msg + "\n")
		}
		return cli.WriteOutput(cmd, []byte(buf.String()))
	}
}
