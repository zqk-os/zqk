package cli

import (
	"encoding/json"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// KindAnnotKeys names the three Cobra annotation keys used for declarative kind validation
// (validate mode, single canonical kind, JSON array for multi-kind count).
type KindAnnotKeys struct {
	Validate  string
	Canonical string
	KindsJSON string
}

// Annotation keys for the zqk object subtree (cmd/zqk/object).
const (
	AnnotationKeyObjectKindValidate  = "zqk.obj.kind_validate"
	AnnotationKeyObjectKindCanonical = "zqk.obj.kind_canonical"
	AnnotationKeyObjectKindsJSON     = "zqk.obj.kinds_json"
)

// Annotation keys for the zqk internal subtree (pkg/zqkcli).
const (
	AnnotationKeyInternalKindValidate  = "zqk.internal.kind_validate"
	AnnotationKeyInternalKindCanonical = "zqk.internal.kind_canonical"
	AnnotationKeyInternalKindsJSON     = "zqk.internal.kinds_json"
)

// Annotation keys for the zqk system subtree (cmd/zqk/system).
const (
	AnnotationKeySystemKindValidate  = "zqk.system.kind_validate"
	AnnotationKeySystemKindCanonical = "zqk.system.kind_canonical"
	AnnotationKeySystemKindsJSON     = "zqk.system.kinds_json"
)

// KindAnnotKeysObject / KindAnnotKeysInternal wire object vs internal annotation namespaces.
var (
	KindAnnotKeysObject = KindAnnotKeys{
		Validate:  AnnotationKeyObjectKindValidate,
		Canonical: AnnotationKeyObjectKindCanonical,
		KindsJSON: AnnotationKeyObjectKindsJSON,
	}
	KindAnnotKeysInternal = KindAnnotKeys{
		Validate:  AnnotationKeyInternalKindValidate,
		Canonical: AnnotationKeyInternalKindCanonical,
		KindsJSON: AnnotationKeyInternalKindsJSON,
	}
	KindAnnotKeysSystem = KindAnnotKeys{
		Validate:  AnnotationKeySystemKindValidate,
		Canonical: AnnotationKeySystemKindCanonical,
		KindsJSON: AnnotationKeySystemKindsJSON,
	}
)

// KindValidateMode values for the validate annotation (shared across object and internal trees).
const (
	KindValidateNone                 = ""
	KindValidatePositional0          = "positional0"
	KindValidatePositional0Opt       = "positional0_opt"
	KindValidateCountArg0            = "count_arg0"
	KindValidateFlagKind             = "flag_kind"
	KindValidateUpdateAllKindFlag    = "update_all_kind_flag"
	KindValidateInternalListOptional = "internal_list_optional"
	KindValidateFieldsParentKind     = "fields_parent_kind"
	// System subtree (cmd/zqk/system).
	KindValidateSystemCompactStream = "system_compact_stream"
	// Object bulk update when --file and positional kind are both set (filter path skips pre-run).
	KindValidateBulkUpdateFileKind = "bulk_update_file_kind"
)

// InternalListOptionalHook implements internal list routing (e.g. lifecycle / object_spec)
// before ResolveAndValidateKindForProject. Return skipResolve true to store canonical without Resolve.
type InternalListOptionalHook func(raw string) (canonical string, skipResolve bool)

// EnsureCmdAnnotations allocates cmd.Annotations when nil.
func EnsureCmdAnnotations(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
}

func clearKindOutputAnnotations(keys KindAnnotKeys, cmd *cobra.Command) {
	if cmd.Annotations == nil {
		return
	}
	delete(cmd.Annotations, keys.Canonical)
	delete(cmd.Annotations, keys.KindsJSON)
}

// ValidateAnnotatedKind runs synonym + spec-index validation before RunE and stores results on cmd.Annotations.
// internalListHook is required only for KindValidateInternalListOptional on internal list (pass nil from object).
func ValidateAnnotatedKind(keys KindAnnotKeys, cmd *cobra.Command, args []string, internalListHook InternalListOptionalHook) error {
	if cmd == nil {
		return nil
	}
	mode := cmd.Annotations[keys.Validate]
	if mode == KindValidateNone || mode == "" {
		return nil
	}

	clearKindOutputAnnotations(keys, cmd)
	projectRoot := ResolveProjectRoot(".")

	switch mode {
	case KindValidatePositional0, KindValidatePositional0Opt:
		if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
			return nil
		}
		k, err := objects.ResolveAndValidateKindForProject(projectRoot, args[0])
		if err != nil {
			return err
		}
		EnsureCmdAnnotations(cmd)
		cmd.Annotations[keys.Canonical] = k
		return nil

	case KindValidateCountArg0:
		return validateCountArg0(keys, cmd, projectRoot, args)

	case KindValidateFlagKind:
		return validateFlagKind(keys, cmd, projectRoot)

	case KindValidateUpdateAllKindFlag:
		return validateUpdateAllKindFlag(keys, cmd, projectRoot)

	case KindValidateInternalListOptional:
		return validateInternalListOptional(keys, cmd, projectRoot, args, internalListHook)

	case KindValidateFieldsParentKind:
		return validateFieldsParentKind(keys, cmd, projectRoot)

	case KindValidateSystemCompactStream:
		return validateSystemCompactStream(keys, cmd, projectRoot)

	case KindValidateBulkUpdateFileKind:
		return validateBulkUpdateFileKind(keys, cmd, projectRoot, args)

	default:
		return nil
	}
}

func validateCountArg0(keys KindAnnotKeys, cmd *cobra.Command, projectRoot string, args []string) error {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		return nil
	}
	arg := args[0]
	var list []string
	var err error
	if strings.Contains(arg, ",") {
		list, err = objects.ResolveAndValidateKindsCommaSeparated(projectRoot, arg)
	} else {
		var k string
		k, err = objects.ResolveAndValidateKindForProject(projectRoot, arg)
		if err != nil {
			return err
		}
		list = []string{k}
	}
	if err != nil {
		return err
	}
	b, mErr := json.Marshal(list)
	if mErr != nil {
		return mErr
	}
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[keys.KindsJSON] = string(b)
	return nil
}

func validateFlagKind(keys KindAnnotKeys, cmd *cobra.Command, projectRoot string) error {
	kind, err := cmd.Flags().GetString("kind")
	if err != nil || strings.TrimSpace(kind) == "" {
		return nil
	}
	k, err := objects.ResolveAndValidateKindForProject(projectRoot, kind)
	if err != nil {
		return err
	}
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[keys.Canonical] = k
	return nil
}

func validateUpdateAllKindFlag(keys KindAnnotKeys, cmd *cobra.Command, projectRoot string) error {
	all, err := cmd.Flags().GetBool("all")
	if err != nil || !all {
		return nil
	}
	kind, err := cmd.Flags().GetString("kind")
	if err != nil || strings.TrimSpace(kind) == "" {
		return nil
	}
	k, err := objects.ResolveAndValidateKindForProject(projectRoot, kind)
	if err != nil {
		return err
	}
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[keys.Canonical] = k
	return nil
}

func validateInternalListOptional(keys KindAnnotKeys, cmd *cobra.Command, projectRoot string, args []string, hook InternalListOptionalHook) error {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		return nil
	}
	raw := strings.TrimSpace(args[0])
	if hook != nil {
		if canon, skip := hook(raw); skip {
			EnsureCmdAnnotations(cmd)
			cmd.Annotations[keys.Canonical] = canon
			return nil
		}
	}
	k, err := objects.ResolveAndValidateKindForProject(projectRoot, raw)
	if err != nil {
		return err
	}
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[keys.Canonical] = k
	return nil
}

func validateFieldsParentKind(keys KindAnnotKeys, cmd *cobra.Command, projectRoot string) error {
	kindArg := ""
	if cmd.Annotations != nil {
		kindArg = strings.TrimSpace(cmd.Annotations[objects.FieldKeyKind])
	}
	if kindArg == "" {
		return nil
	}
	k, err := objects.ResolveAndValidateKindForProject(projectRoot, kindArg)
	if err != nil {
		return err
	}
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[keys.Canonical] = k
	return nil
}

func validateSystemCompactStream(keys KindAnnotKeys, cmd *cobra.Command, projectRoot string) error {
	all, err := cmd.Flags().GetBool("all")
	if err != nil || all {
		return nil
	}
	kind, err := cmd.Flags().GetString("kind")
	if err != nil || strings.TrimSpace(kind) == "" {
		return nil
	}
	k, err := objects.ResolveAndValidateKindForProject(projectRoot, kind)
	if err != nil {
		return err
	}
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[keys.Canonical] = k
	return nil
}

func validateBulkUpdateFileKind(keys KindAnnotKeys, cmd *cobra.Command, projectRoot string, args []string) error {
	filePath, err := cmd.Flags().GetString("file")
	if err != nil || strings.TrimSpace(filePath) == "" {
		return nil
	}
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		return nil
	}
	k, err := objects.ResolveAndValidateKindForProject(projectRoot, args[0])
	if err != nil {
		return err
	}
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[keys.Canonical] = k
	return nil
}

// KindCanonicalFromPRERun reads the validated single-kind annotation set by ValidateAnnotatedKind.
func KindCanonicalFromPRERun(keys KindAnnotKeys, cmd *cobra.Command) (string, bool) {
	if cmd == nil || cmd.Annotations == nil {
		return "", false
	}
	k := strings.TrimSpace(cmd.Annotations[keys.Canonical])
	return k, k != ""
}

// KindsListFromPRERun reads the JSON kinds array set for count commands.
func KindsListFromPRERun(keys KindAnnotKeys, cmd *cobra.Command) ([]string, bool) {
	if cmd == nil || cmd.Annotations == nil {
		return nil, false
	}
	s := cmd.Annotations[keys.KindsJSON]
	if strings.TrimSpace(s) == "" {
		return nil, false
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil || len(out) == 0 {
		return nil, false
	}
	return out, true
}
