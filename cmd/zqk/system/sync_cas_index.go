package system

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
)

// NewSyncCASIndexCmd creates a command to sync the CAS index from an existing hash-addressed file.
// Use when the index points to a missing or wrong hash (e.g. after a file rename) and the correct file exists on disk.
func NewSyncCASIndexCmd() *cobra.Command {
	var filePath string
	var dryRun bool

	exampleCASFile := filepath.Join(paths.ProcessBacklogDir, "<hash>.yaml")
	cmdLong := fmt.Sprintf(`Sync the content-addressable storage index so an object ID maps to the hash of an existing file.
Use when the index is out of sync (e.g. file was renamed to match content hash) and the correct file exists on disk.

Examples:
  # Update index for a single file
  %s system sync-cas-index --file %s
  # Preview without writing
  %s system sync-cas-index --file %s --dry-run`,
		paths.CLICommandName, exampleCASFile, paths.CLICommandName, exampleCASFile)

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSyncCasIndexCommandBuilder(), &cobra.Command{
		Use:   "sync-cas-index",
		Short: "Sync CAS index from an existing hash-addressed file",
		Long:  cmdLong,
	})
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, _ []string) error {
		if filePath == emptyValue {
			return cli.Guard(cmd).Require(false, "--file is required").Return()
		}
		projectRoot := ProjectRootOrResolve("")
		if projectRoot == emptyValue {
			return cli.Guard(cmd).Require(false, "project root not found (run from repo root or set ZQK_PROJECT_ROOT)").Return()
		}
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return cli.Guard(cmd).Err(err).Wrapf("resolve file path: %w").Return()
		}
		return runSyncCASIndex(cmd, projectRoot, absPath, dryRun)
	})
	cmd.Flags().StringVar(&filePath, "file", "", "Path to the hash-addressed YAML file (e.g. "+filepath.Join(paths.ProcessBacklogDir, "<hash>.yaml")+")")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without updating the index")
	cli.AddCommonFlags(cmd)
	return cmd
}

func runSyncCASIndex(cmd *cobra.Command, projectRoot, absPath string, dryRun bool) error {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("processor: %w").Return()
	}

	content, err := fileutil.ReadFile(absPath)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("read file: %w").Return()
	}
	var obj map[string]any
	if err := yaml.Unmarshal(content, &obj); err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("parse YAML: %w").Return()
	}
	objectID, _ := obj[objects.FieldKeyID].(string)
	if objectID == emptyValue {
		return cli.Guard(cmd).Require(false, "file has no id field").Return()
	}

	hashBytes := sha256.Sum256(content)
	contentHash := hex.EncodeToString(hashBytes[:])

	kindDir := filepath.Dir(absPath)
	dirName := filepath.Base(kindDir)
	kind := objects.GetKindFromDirectory(dirName)
	if kind == emptyValue {
		return cli.Guard(cmd).Err(errfmt.Errorf("unknown kind for directory %q", dirName)).Return()
	}

	// Filename should match content hash for CAS
	base := filepath.Base(absPath)
	expectedName := contentHash + filepath.Ext(base)
	if base != expectedName {
		logging.FluentEvent(proc.Logger()).Warn("Filename does not match content hash").
			File(base).
			String("content_hash", contentHash[:16]+"...").
			Log()
	}

	if dryRun {
		msg := fmt.Sprintf("DRY RUN: would set CAS index %s -> %s (kind: %s)", objectID, contentHash[:16]+"...", kind)
		logging.FluentEvent(proc.Logger()).Info(msg).Log()
		return cli.WriteOutput(cmd, []byte(msg+"\n"))
	}

	var cas *storage.ContentAddressableStorage
	if c, ok := getCachedCASForKind(proc.OperationContext(), projectRoot, kind); ok {
		cas = c
	}
	if cas == nil {
		cas = storage.NewContentAddressableStorage(kindDir, kind)
	}
	if err := cas.GetIndex().SetMapping(objectID, contentHash); err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("set CAS index mapping: %w").Return()
	}
	logging.Fluent(logger).Info("Synced CAS index").
		ObjectID(objectID).
		Kind(kind).
		String("hash", contentHash[:16]+"...").
		Log()
	msg := fmt.Sprintf("Synced CAS index for %s (kind: %s)\n", objectID, kind)
	return cli.WriteOutput(cmd, []byte(msg))
}
