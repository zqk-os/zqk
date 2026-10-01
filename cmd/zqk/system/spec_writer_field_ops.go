package system

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// getStorageProviderForSpecs gets or creates a storage provider for spec metrics via StorageFactory
func getStorageProviderForSpecs(projectRoot string) (storage.ObjectStorageProvider, error) {
	factory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
	if err != nil {
		return nil, err
	}
	return factory.GetStorage(), nil
}

// runFieldOperation handles field-level operations (define/create/add, modify, deprecate, archive, delete)
func runFieldOperation(
	cmd *cobra.Command,
	projectRoot string,
	fieldName string,
	operation string,
	breakingChange bool,
	reason string,
	migrationNotes string,
	replacedBy string,
	dryRun bool,
	_ bool,
	validateAfter bool,
) error {
	operation = ResolveSpecFieldOperation(operation)

	ctx := cli.GetContext(cmd)
	logger := logging.GetLoggerFromProfile(ctx.Profile)

	// Get ontology from args or prompt
	args := cmd.Flags().Args()
	if len(args) == 0 {
		return errfmt.Errorf("ontology required (e.g., backlog_item)")
	}
	ontology := args[0]

	// Get current user (system account for spec field ops)
	currentUser := pkgctx.SystemAccountID

	// Get storage provider for metrics
	// For now, we'll need to get it from context or create a file storage
	// In a full implementation, this would come from the CLI context
	storageProvider, err := getStorageProviderForSpecs(projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage provider").Wrap(err)
	}

	// Create spec writer
	specWriter, err := NewSpecWriter(projectRoot, currentUser, storageProvider)
	if err != nil {
		return errfmt.Newf("failed to create spec writer").Wrap(err)
	}

	// Load current spec to get field definition if modifying
	var fieldDef map[string]any
	if FieldOpRequiresDefinitionFile(operation) {
		// For create/modify, we need the field definition
		// This could come from a file, stdin, or be built interactively
		// For now, we'll require it to be provided via a file or prompt
		fieldDefFile := fmt.Sprintf("%s_field_%s.yaml", ontology, fieldName)
		if _, err := fileutil.Stat(fieldDefFile); err == nil {
			// Load from file
			data, err := fileutil.ReadFile(fieldDefFile)
			if err != nil {
				return errfmt.Newf("failed to read field definition file").Wrap(err)
			}
			if err := yaml.Unmarshal(data, &fieldDef); err != nil {
				return errfmt.Newf("failed to parse field definition").Wrap(err)
			}
		} else {
			// For now, return error - in future, could prompt interactively
			return errfmt.Errorf("field definition file not found: %s. Create this file with the field definition, or use interactive mode (coming soon)", fieldDefFile)
		}
	}

	// Create field operation
	fieldOp := FieldOperation{
		Operation:      operation,
		FieldName:      fieldName,
		FieldDef:       fieldDef,
		BreakingChange: breakingChange,
		Reason:         reason,
		MigrationNotes: migrationNotes,
		ReplacedBy:     replacedBy,
	}

	if dryRun {
		logging.Fluent(logger).Info("Dry run - would apply field operation").
			String("ontology", ontology).
			String("field", fieldName).
			String("operation", operation).
			Log()
		return nil
	}

	// Apply operation
	if err := specWriter.ApplyFieldOperations(ontology, []FieldOperation{fieldOp}); err != nil {
		return errfmt.Newf("failed to apply field operation").Wrap(err)
	}

	if _, err := objects.RefreshMaterializedSpecIndex(projectRoot); err != nil {
		return errfmt.Newf("refresh materialized spec index").Wrap(err)
	}
	InvalidateDescriptorReadModelCache(projectRoot)

	// Optional: same field-vetting path as update-specs --validate on file batches (LoadSpecAndValidate).
	if validateAfter {
		loader := objects.GetGlobalSpecLoader()
		loader.InvalidateSpec(ontology)
		stdCtx := pkgctx.NewSystemContext()
		_, valErrs, err := objects.LoadSpecAndValidate(stdCtx, loader, ontology+".yaml", nil)
		if err != nil {
			return errfmt.Newf("reload spec after field operation for validation").Wrap(err)
		}
		if len(valErrs) > 0 {
			first := valErrs[0]
			return errfmt.Errorf(
				"spec validation failed (%d issue(s); e.g. field %q: %s). Fix checklist or omit --validate",
				len(valErrs), first.Field, first.Message)
		}
	}

	logging.Fluent(logger).Info("Field operation completed").
		String("ontology", ontology).
		String("field", fieldName).
		String("operation", operation).
		Log()

	return nil
}
