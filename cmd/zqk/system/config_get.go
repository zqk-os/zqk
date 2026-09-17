package system

import (
	"path/filepath"
	"strings"

	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
)

// NewConfigGetCmd returns a command that prints a project config value by dot-separated key.
// Used by scripts to read settings like logging.error_log_output (separate|combined).
func NewConfigGetCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemConfigGetCommandBuilder(), &cobra.Command{
		Use:    "config-get [key]",
		Short:  "Print a project config value by key (e.g. logging.error_log_output)",
		Hidden: true,
	})
	cli.BindAsyncProgress(cmd, runConfigGet)
	return cmd
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errfmt.Errorf("usage: config-get <key>")
	}
	key := args[0]
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}
	cfg, err := loadProjectConfig(projectRoot)
	if err != nil {
		return err
	}
	val, ok := getNestedString(cfg, key)
	if !ok || val == emptyValue {
		return errfmt.Errorf("key not found or not a string: %s", key)
	}
	return cli.WriteOutput(cmd, []byte(val+"\n"))
}

func loadProjectConfig(projectRoot string) (map[string]any, error) {
	for _, rel := range []string{
		filepath.Join(paths.ProjectDataDir, "config", "config.yaml"),
		filepath.Join(paths.ProjectDataDir, "config.yaml"),
	} {
		p := filepath.Join(projectRoot, rel)
		data, err := fileutil.ReadFile(p)
		if err != nil {
			if fileutil.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var cfg map[string]any
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	return nil, errfmt.Errorf("no project config file found under %s", projectRoot)
}

func getNestedString(m map[string]any, key string) (string, bool) {
	parts := strings.Split(key, ".")
	var current any = m
	for _, part := range parts {
		if current == nil {
			return "", false
		}
		if m, ok := current.(map[string]any); ok {
			current = m[part]
			continue
		}
		return "", false
	}
	if s, ok := current.(string); ok {
		return s, true
	}
	return "", false
}
