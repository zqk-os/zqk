package internal

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/objects"
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

	cli.BindAsyncProgress(cmd, runInternalCreate)
	cmd.Flags().String("file", "", "Path to YAML file containing object data (required)")
	cmd.Flags().String("data", "", "Inline YAML data for the object")
	cmd.Flags().Bool("dry-run", false, "Show what would be created without actually creating it")

	ensureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKindValidate] = KindValidatePositional0

	return cli.FinalizeCommand(cmd, helpBuilder)
}

func runInternalCreate(cmd *cobra.Command, args []string) error {
	proc, err := newInternalProcessor(cmd)
	if err != nil {
		return err
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
