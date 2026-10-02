package object

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

func showSingletonOrFirstObject(cmd *cobra.Command, proc *cli.Processor, args []string, kind, singularDesc, pluralDesc string) error {
	id := ""
	if len(args) > 0 {
		id = args[0]
	}

	if id == emptyValue {
		secCtx := pkgctx.NewSystemSecurityContext()
		storageCtx := proc.StorageContext()
		filter := storage.ListFilter{
			Kind:    kind,
			Filters: map[string]any{},
			SortBy:  objects.FieldKeyID,
			SortAsc: true,
			Limit:   1,
		}
		result, err := proc.Storage().List(cmd.Context(), secCtx, storageCtx, filter)
		if err != nil {
			return cli.Guard(cmd).Err(err).Wrapf(fmt.Sprintf("failed to list %s: %%w", pluralDesc)).Return()
		}
		if len(result.Objects) == 0 {
			logging.FluentEvent(proc.Logger()).Info(fmt.Sprintf("No %s found", pluralDesc)).Log()
			return cli.Guard(cmd).Err(errfmt.Errorf("no %s found", pluralDesc)).Return()
		}
		id, _ = result.Objects[0][objects.FieldKeyID].(string)
		if id == emptyValue {
			return cli.Guard(cmd).Err(errfmt.Errorf("%s object missing id", singularDesc)).Return()
		}
	}

	logging.FluentEvent(proc.Logger()).Debug(fmt.Sprintf("Reading %s", singularDesc)).
		ObjectID(id).
		Log()
	obj, err := proc.Storage().Read(cmd.Context(), proc.SecurityContext(), id)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error(fmt.Sprintf("Failed to read %s", singularDesc), err).
			ObjectID(id).
			Log()
		return cli.Guard(cmd).Err(err).Wrapf(fmt.Sprintf("failed to read %s: %%w", singularDesc)).Return()
	}

	// BLI-642: field-level permissions — filter to fields the security context can read (same as object get)
	if proc.ProjectRoot() != emptyValue && !skipObjectGetFieldACL(proc.SecurityContext()) {
		specsDir := filepath.Join(proc.ProjectRoot(), paths.ProcessInternalObjectSpecsDir)
		loader := objects.NewSpecLoader(specsDir)
		sac := mcp.NewSpecAccessControlFromObjectsLoader(loader)
		if sac != nil {
			obj = sac.FilterObjectFields(obj, proc.SecurityContext(), "read")
		}
	}

	logging.FluentEvent(proc.Logger()).Debug(fmt.Sprintf("%s read successfully", singularDesc)).
		ObjectID(id).
		Log()
	return cli.FormatOutput(cmd, obj)
}
