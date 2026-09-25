package spec

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/appledouble"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// NewSpecListCmd creates the "spec list" subcommand.
func NewSpecListCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSpecListCommandBuilder(), &cobra.Command{
		Use:   "list",
		Short: "List available object specifications",
		Long:  "List object_spec objects from storage (or bundled specs when using file backend).",
	})
	cmd.Aliases = []string{"ls"}
	cli.BindAsyncProgress(cmd, runSpecList)
	cli.AddCommonFlags(cmd)
	return cmd
}

func runSpecList(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var err error
		_ = err

		storageCtx := proc.StorageContext()
		listFilter := storage.ListFilter{
			Kind:    objects.KindObjectSpec,
			Filters: map[string]any{},
			Limit:   0,
		}

		result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), storageCtx, listFilter)
		if err != nil || len(result.Objects) == 0 {
			// File backend often has no storage for object_spec; fall back to scanning .zqk/specs/objects
			result, err = listSpecsFromFiles(proc.ProjectRoot())
			if err != nil {
				return cli.Guard(cmd).Err(err).Wrapf("failed to list object specs: %w").Return()
			}
		}

		data := map[string]any{
			"objects": result.Objects,
			"meta":    result.Meta,
		}
		if err := cli.FormatOutput(cmd, data); err != nil {
			return cli.Guard(cmd).Err(err).Wrapf("format output: %w").Return()
		}
		return nil
	})(cmd, nil)
}

// listSpecsFromFiles scans .zqk/specs/objects for YAML spec files (file-backend fallback).
func listSpecsFromFiles(projectRoot string) (*storage.QueryResult, error) {
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := fileutil.Stat(specsDir); err != nil {
		if fileutil.IsNotExist(err) {
			return &storage.QueryResult{
				Objects: []map[string]any{},
				Meta:    map[string]any{"total_count": 0},
			}, nil
		}
		return nil, errfmt.Newf("failed to access object_specs directory").Wrap(err)
	}

	var specObjects []map[string]any
	err := filepath.Walk(specsDir, func(path string, info fileutil.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil //nolint:nilerr // skip unreadable files
		}
		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".yaml") && !strings.HasSuffix(info.Name(), ".yml") {
			return nil
		}
		if strings.HasPrefix(info.Name(), "_") {
			return nil
		}

		data, readErr := fileutil.ReadFile(path)
		if readErr != nil {
			return nil //nolint:nilerr // skip files that cannot be read
		}
		var specDef map[string]any
		if parseErr := yaml.Unmarshal(data, &specDef); parseErr != nil {
			return nil //nolint:nilerr // skip unparseable YAML
		}
		ontology, _ := specDef[objects.FieldKeyOntology].(string)
		if ontology == emptyValue {
			return nil
		}

		id := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
		title := fmt.Sprintf("%s Specification", ontology)
		if desc, ok := specDef[objects.FieldKeyDescription].(string); ok && desc != emptyValue {
			first := strings.TrimSpace(strings.Split(desc, "\n")[0])
			if first != emptyValue {
				if len(first) > 80 {
					title = first[:77] + "..."
				} else {
					title = first
				}
			}
		}

		obj := map[string]any{
			objects.FieldKeyID:       id,
			objects.FieldKeyKind:     objects.KindObjectSpec,
			objects.FieldKeyTitle:    title,
			objects.FieldKeyOntology: ontology,
			objects.FieldKeyFilePath: path,
		}
		if v, ok := specDef[objects.FieldKeySchemaVersion].(string); ok {
			obj[objects.FieldKeySchemaVersion] = v
		}
		if v, ok := specDef[objects.FieldKeyVisibility].(string); ok {
			obj[objects.FieldKeyVisibility] = v
		}
		specObjects = append(specObjects, obj)
		return nil
	})
	if err != nil {
		return nil, errfmt.Newf("failed to scan object_specs directory").Wrap(err)
	}

	return &storage.QueryResult{
		Objects: specObjects,
		Meta:    map[string]any{"total_count": len(specObjects)},
	}, nil
}
