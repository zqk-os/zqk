package internal

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	objkeys "github.com/zqk-os/zqk/pkg/objects"
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

func newInternalProcessorWithRoot(cmd *cobra.Command) (*cli.Processor, string, error) {
	proc, err := newInternalProcessor(cmd)
	if err != nil {
		return nil, "", err
	}
	return proc, proc.ProjectRoot(), nil
}

func resolveInternalKindAndProcessor(cmd *cobra.Command, args []string) (*cli.Processor, string, error) {
	proc, root, err := newInternalProcessorWithRoot(cmd)
	if err != nil {
		return nil, "", err
	}
	kind, ok := kindCanonicalFromInternalPRERun(cmd)
	if !ok {
		var rerr error
		kind, rerr = objkeys.ResolveAndValidateKindForProject(root, args[0])
		if rerr != nil {
			return nil, "", rerr
		}
	}
	return proc, kind, nil
}
