package object

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Annotation keys — aliases for declarative kind validation on the object command subtree.
const (
	AnnotationKindValidate  = cli.AnnotationKeyObjectKindValidate
	AnnotationKindCanonical = cli.AnnotationKeyObjectKindCanonical
	AnnotationKindsJSON     = cli.AnnotationKeyObjectKindsJSON
)

// KindValidateMode values (see internal/cli.KindValidateMode).
const (
	KindValidateNone               = cli.KindValidateNone
	KindValidatePositional0        = cli.KindValidatePositional0
	KindValidatePositional0Opt     = cli.KindValidatePositional0Opt
	KindValidateCountArg0          = cli.KindValidateCountArg0
	KindValidateFlagKind           = cli.KindValidateFlagKind
	KindValidateUpdateAllKindFlag  = cli.KindValidateUpdateAllKindFlag
	KindValidateBulkUpdateFileKind = cli.KindValidateBulkUpdateFileKind
	KindValidateFieldsParentKind   = cli.KindValidateFieldsParentKind
)

func ensureCmdAnnotations(cmd *cobra.Command) {
	cli.EnsureCmdAnnotations(cmd)
}

func configureKindPositionalValidation(cmd *cobra.Command, withCompletion bool) {
	if withCompletion {
		cmd.ValidArgsFunction = kindCompletion
	}
	ensureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKindValidate] = KindValidatePositional0
}

func resolvePositional0Kind(cmd *cobra.Command, proc *cli.Processor, args []string, requiredErr string) (string, error) {
	kind, ok := kindCanonicalFromPRERun(cmd)
	if ok {
		return kind, nil
	}
	if len(args) == 0 {
		if requiredErr != "" {
			return "", errors.New(requiredErr)
		}
		return "", nil
	}
	return objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), args[0])
}

func validateAnnotatedObjectKind(cmd *cobra.Command, args []string) error {
	return cli.ValidateAnnotatedKind(cli.KindAnnotKeysObject, cmd, args, nil)
}

func kindCanonicalFromPRERun(cmd *cobra.Command) (string, bool) {
	return cli.KindCanonicalFromPRERun(cli.KindAnnotKeysObject, cmd)
}

func kindsListFromPRERun(cmd *cobra.Command) ([]string, bool) {
	return cli.KindsListFromPRERun(cli.KindAnnotKeysObject, cmd)
}
