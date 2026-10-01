package internal

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

const (
	AnnotationKindValidate  = cli.AnnotationKeyInternalKindValidate
	AnnotationKindCanonical = cli.AnnotationKeyInternalKindCanonical
	AnnotationKindsJSON     = cli.AnnotationKeyInternalKindsJSON
)

const (
	KindValidateNone                 = cli.KindValidateNone
	KindValidatePositional0          = cli.KindValidatePositional0
	KindValidateInternalListOptional = cli.KindValidateInternalListOptional
	KindValidateCountArg0            = cli.KindValidateCountArg0
	KindValidateFieldsParentKind     = cli.KindValidateFieldsParentKind
)

func ensureCmdAnnotations(cmd *cobra.Command) {
	cli.EnsureCmdAnnotations(cmd)
}

func validateAnnotatedInternalKind(cmd *cobra.Command, args []string) error {
	return cli.ValidateAnnotatedKind(cli.KindAnnotKeysInternal, cmd, args, internalListOptionalHook)
}

func internalListOptionalHook(raw string) (canonical string, skipResolve bool) {
	raw = strings.TrimSpace(raw)
	if raw == internalKindLifecycle || raw == internalKindObjectSpec {
		return raw, true
	}
	return "", false
}

func kindCanonicalFromInternalPRERun(cmd *cobra.Command) (string, bool) {
	return cli.KindCanonicalFromPRERun(cli.KindAnnotKeysInternal, cmd)
}

func kindsListFromInternalPRERun(cmd *cobra.Command) ([]string, bool) {
	return cli.KindsListFromPRERun(cli.KindAnnotKeysInternal, cmd)
}
