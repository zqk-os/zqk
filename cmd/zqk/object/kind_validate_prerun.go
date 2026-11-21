package object

import (
	"errors"
	"strings"

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

// resolveRequiredObjectKind resolves and validates the canonical object kind from
// PreRun annotations, an optional --kind flag (when allowKindFlag is true), or positional args[0].
func resolveRequiredObjectKind(cmd *cobra.Command, proc *cli.Processor, args []string, allowKindFlag bool, missingMsg string) (string, error) {
	if nk, ok := kindCanonicalFromPRERun(cmd); ok && nk != "" {
		return nk, nil
	}

	rawKind := ""
	if allowKindFlag && cmd != nil && cmd.Flags() != nil {
		kindFlag, _ := cmd.Flags().GetString("kind")
		rawKind = strings.TrimSpace(kindFlag)
	}
	if rawKind == emptyValue && len(args) > 0 {
		rawKind = strings.TrimSpace(args[0])
	}
	if rawKind == emptyValue {
		return "", cli.Guard(cmd).Require(false, missingMsg).Return()
	}

	kind, err := objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), rawKind)
	if err != nil {
		return "", cli.Guard(cmd).Err(err).Return()
	}
	return kind, nil
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
