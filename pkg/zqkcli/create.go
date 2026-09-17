package internal

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewInternalCreateCmd creates a create command for internal objects
func NewInternalCreateCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Create an internal object (admin only)",
		"Create an internal object with admin privileges.",
		"",
		"This command allows creating objects that may be internal or built-in instances,",
		"including object specifications, lifecycles, templates, and other system objects.",
	).
		AddExample("Create an object specification", "%s internal create object_spec --file my_spec.yaml").
		AddExample("Create an internal lifecycle definition", "%s internal create lifecycle --file lifecycle.yaml").
		AddExample("Create a kind synonym", "%s internal create kind_synonym --file synonym.yaml").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:  "create <kind> --file <file>",
		Args: cobra.ExactArgs(1),
	}

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.BindAsyncProgress(cmd, runInternalCreate)
	cli.AddCommonFlags(cmd)
	cmd.Flags().String("file", "", "Path to YAML file containing object data (required)")
	cmd.Flags().String("data", "", "Inline YAML data for the object")
	cmd.Flags().Bool("dry-run", false, "Show what would be created without actually creating it")

	ensureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKindValidate] = KindValidatePositional0

	return cmd
}

func runInternalCreate(cmd *cobra.Command, args []string) error {
	// Create processor (handles context, storage, security, logging)
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return errfmt.Newf("failed to create processor").Wrap(err)
	}

	kind, ok := kindCanonicalFromInternalPRERun(cmd)
	if !ok {
		var rerr error
		kind, rerr = objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), args[0])
		if rerr != nil {
			return rerr
		}
	}

	// Read object data
	objData, filePath, err := readObjectData(cmd, proc, kind)
	if err != nil {
		return err
	}

	// Validate and prepare object
	if err := validateAndPrepareObject(objData, kind, proc); err != nil {
		return err
	}

	// Check for dry-run
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		dryRun = false
	}
	if dryRun {
		return handleDryRunCreate(cmd, proc, objData, kind)
	}

	// Create object
	_, err = createInternalObject(proc, objData, kind)
	if err != nil {
		return err
	}

	if filePath != emptyValue {
		_ = cli.ClearLastDraftPointerIfPath(proc.ProjectRoot(), filePath)
	}

	// Output success message using shared utility
	msg := clipkg.FormatCreateSuccessMessage(objData, kind, "Internal object", proc.Logger())
	return cli.WriteOutput(cmd, []byte(msg))
}
