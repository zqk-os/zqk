package organizational

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// NewRecordChangeCmd creates the record-change command from the generated builder.
func NewRecordChangeCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewOrganizationalRecordChangeCommandBuilder()
	cli.BindAsyncProgress(cmd, runRecordChange)
	return cmd
}

func runRecordChange(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		id, _ := cmd.Flags().GetString("id")
		changeType, _ := cmd.Flags().GetString("change-type")
		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		affectedObjectsPath, _ := cmd.Flags().GetString("affected-objects")
		changeDateStr, _ := cmd.Flags().GetString("change-date")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if id == emptyValue || changeType == emptyValue {
			return errfmt.Errorf("--id and --change-type are required")
		}

		if title == emptyValue {
			title = changeType
		}

		obj := map[string]any{
			objects.FieldKeyID:                 id,
			objects.FieldKeyKind:               objects.KindOrganizationalChange,
			objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:              title,
			objects.FieldKeyStatus:             objects.ObjectStatusProposed,
			objects.FieldKeyChangeType:         changeType,
			objects.FieldKeyChangeDescription:  description,
			objects.FieldKeyAffectedObjects:    map[string]any{},
			objects.FieldKeyImpactAnalysisRefs: []string{},
		}

		if changeDateStr != emptyValue {
			obj[objects.FieldKeyChangeDate] = changeDateStr
		} else {
			obj[objects.FieldKeyChangeDate] = zqktime.NowRFC3339UTC()
		}

		if affectedObjectsPath != emptyValue {
			data, err := fileutil.ReadFile(affectedObjectsPath)
			if err != nil {
				logging.FluentEvent(proc.Logger()).Error("Failed to read affected-objects file", err).File(affectedObjectsPath).Log()
				return errfmt.Newf("failed to read affected-objects file").Wrap(err)
			}
			var m map[string]any
			if err := yaml.Unmarshal(data, &m); err != nil {
				if err := json.Unmarshal(data, &m); err != nil {
					return errfmt.Newf("affected-objects file must be YAML or JSON map").Wrap(err)
				}
			}
			if m != nil {
				obj[objects.FieldKeyAffectedObjects] = m
			}
		}

		if dryRun {
			logging.FluentEvent(proc.Logger()).Info("Record-change dry-run: would create organizational_change").
				ObjectID(id).
				String("change_type", changeType).
				String("title", title).
				Log()
			return nil
		}

		opCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), id, objects.KindOrganizationalChange, "")
		if err := proc.Storage().Create(opCtx, proc.SecurityContext(), obj); err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to create organizational_change", err).ObjectID(id).Log()
			return errfmt.Newf("failed to create organizational_change").Wrap(err)
		}

		logging.FluentEvent(proc.Logger()).Info("Organizational change recorded").
			ObjectID(id).
			String("change_type", changeType).
			Log()
		msg := fmt.Sprintf("Organizational change recorded: %s\nRun 'zqk organizational analyze-impact --change %s' to analyze impact.\n", id, id)
		return cli.WriteOutput(cmd, []byte(msg))
	})(cmd, args)
}
