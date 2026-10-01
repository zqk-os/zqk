package organizational

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Allowed kinds for organizational structure sync (domain:organizational).
var orgSyncAllowedKinds = map[string]bool{
	objects.KindOrganization:         true,
	objects.KindDivision:             true,
	objects.KindDepartment:           true,
	objects.KindTeam:                 true,
	objects.KindPartnership:          true,
	objects.KindOrganizationalChange: true,
}

// NewSyncCmd creates the organizational sync command from the generated builder.
func NewSyncCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewOrganizationalSyncCommandBuilder()
	cli.BindAsyncProgress(cmd, runSync)
	return cmd
}

func runSync(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		filePath, _ := cmd.Flags().GetString("file")
		inputFormat, _ := cmd.Flags().GetString("input-format")
		modeStr, _ := cmd.Flags().GetString("mode")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		if inputFormat == emptyValue {
			inputFormat = "yaml"
		}
		if modeStr == emptyValue {
			modeStr = "upsert"
		}

		if filePath == emptyValue {
			return errfmt.Errorf("--file is required")
		}

		data, err := fileutil.ReadFile(filePath)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to read file", err).File(filePath).Log()
			return errfmt.Newf("failed to read file").Wrap(err)
		}

		format := storage.ExportFormat(inputFormat)
		if format != storage.ExportFormatYAML && format != storage.ExportFormatJSON {
			return errfmt.Errorf("unsupported format %q (use yaml or json)", inputFormat)
		}

		parsedRows, err := unmarshalOrgSync(data, format)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to parse file", err).Log()
			return errfmt.Newf("failed to parse file").Wrap(err)
		}

		var validated []map[string]any
		for i, obj := range parsedRows {
			kind, _ := obj[objects.FieldKeyKind].(string)
			if kind == emptyValue {
				logging.FluentEvent(proc.Logger()).Warn("Object missing kind").Index(i).Log()
				if !continueOnError {
					return errfmt.Errorf("object at index %d missing kind", i)
				}
				continue
			}
			if !orgSyncAllowedKinds[kind] {
				logging.FluentEvent(proc.Logger()).Warn("Object kind not allowed for organizational sync").
					Kind(kind).
					Int("index", i).
					Log()
				if !continueOnError {
					return errfmt.Errorf("kind %q is not allowed for organizational sync (allowed: %s)", kind, allowedKindsList())
				}
				continue
			}
			validated = append(validated, obj)
		}

		if len(validated) == 0 {
			proc.Logger().LogInfo("No valid organizational objects to sync")
			return nil
		}

		dataToImport, err := marshalOrgSync(validated, format)
		if err != nil {
			return errfmt.Newf("re-marshal for import").Wrap(err)
		}

		importMode := storage.ImportModeCreateOnly
		if modeStr == "upsert" {
			importMode = storage.ImportModeUpsert
		}

		options := storage.ImportOptions{
			Mode:            importMode,
			ValidateOnly:    dryRun,
			ContinueOnError: continueOnError,
		}

		result, err := storage.ImportObjects(
			proc.OperationContext(),
			proc.Storage(),
			proc.SecurityContext(),
			dataToImport,
			format,
			options,
		)
		if err != nil {
			proc.Logger().LogError("Organizational sync failed", err)
			return errfmt.Newf("organizational sync failed").Wrap(err)
		}

		if dryRun {
			logging.FluentEvent(proc.Logger()).Info("Organizational sync dry-run completed").
				Int("objects_validated", len(validated)).
				Log()
			return nil
		}

		logging.FluentEvent(proc.Logger()).Info("Organizational sync completed").
			Int("created", result.Created).
			Int("updated", result.Updated).
			Int("skipped", result.Skipped).
			Int("failed", result.Failed).
			Log()
		if result.Failed > 0 && len(result.Errors) > 0 {
			for _, e := range result.Errors {
				logging.FluentEvent(proc.Logger()).Warn("Sync error").
					ObjectID(e.ID).
					Int("index", e.Index).
					String("message", e.Message).
					Log()
			}
		}
		return nil
	})(cmd, args)
}

func allowedKindsList() string {
	var list []string
	for k := range orgSyncAllowedKinds {
		list = append(list, k)
	}
	return strings.Join(list, ", ")
}

func unmarshalOrgSync(data []byte, format storage.ExportFormat) ([]map[string]any, error) {
	var objects []map[string]any
	switch strings.ToLower(string(format)) {
	case string(storage.ExportFormatYAML), "":
		if err := yaml.Unmarshal(data, &objects); err != nil {
			return nil, err
		}
	case string(storage.ExportFormatJSON):
		if err := json.Unmarshal(data, &objects); err != nil {
			return nil, err
		}
	default:
		return nil, errfmt.Errorf("unsupported format: %s", format)
	}
	if objects == nil {
		objects = []map[string]any{}
	}
	return objects, nil
}

func marshalOrgSync(objects []map[string]any, format storage.ExportFormat) ([]byte, error) {
	switch strings.ToLower(string(format)) {
	case string(storage.ExportFormatYAML), "":
		return yaml.Marshal(objects)
	case string(storage.ExportFormatJSON):
		return json.Marshal(objects)
	default:
		return nil, errfmt.Errorf("unsupported format: %s", format)
	}
}
