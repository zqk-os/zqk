package object

import (
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/spf13/cobra"
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

func validateAnnotatedObjectKind(cmd *cobra.Command, args []string) error {
	return cli.ValidateAnnotatedKind(cli.KindAnnotKeysObject, cmd, args, nil)
}

func kindCanonicalFromPRERun(cmd *cobra.Command) (string, bool) {
	return cli.KindCanonicalFromPRERun(cli.KindAnnotKeysObject, cmd)
}

func kindsListFromPRERun(cmd *cobra.Command) ([]string, bool) {
	return cli.KindsListFromPRERun(cli.KindAnnotKeysObject, cmd)
}
