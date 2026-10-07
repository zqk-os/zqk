package cli

import (
	"testing"

	pkgcli "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestValidateAnnotatedKind_noAnnotation_isNoOp(t *testing.T) {
	t.Parallel()
	cmd := pkgcli.NewCommandBuilder("x").Build()
	if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, []string{"any"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBulkUpdateFileKind_skipsWithoutFileFlag(t *testing.T) {
	t.Parallel()
	cmd := pkgcli.NewCommandBuilder("bulk-update").Build()
	cmd.Flags().String("file", "", "")
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKeyObjectKindValidate] = KindValidateBulkUpdateFileKind

	if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, []string{"some_kind"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := KindCanonicalFromPRERun(KindAnnotKeysObject, cmd); ok {
		t.Fatal("expected no canonical kind when --file is unset")
	}
}

func TestValidateSystemCompactStream_skipsWhenAll(t *testing.T) {
	t.Parallel()
	cmd := pkgcli.NewCommandBuilder("compact-stream-state").Build()
	cmd.Flags().Bool("all", false, "")
	cmd.Flags().String("kind", "", "")
	if err := cmd.Flags().Set("all", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("kind", "ignored"); err != nil {
		t.Fatal(err)
	}
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKeySystemKindValidate] = KindValidateSystemCompactStream

	if err := ValidateAnnotatedKind(KindAnnotKeysSystem, cmd, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := KindCanonicalFromPRERun(KindAnnotKeysSystem, cmd); ok {
		t.Fatal("expected no canonical kind when --all is set")
	}
}

func TestValidateFieldsParentKind_storesCanonical(t *testing.T) {
	t.Parallel()
	cmd := pkgcli.NewCommandBuilder("fields").Build()
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[objects.FieldKeyKind] = "backlog_item"
	cmd.Annotations[AnnotationKeyObjectKindValidate] = KindValidateFieldsParentKind

	if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	k, ok := KindCanonicalFromPRERun(KindAnnotKeysObject, cmd)
	if !ok || k == "" {
		t.Fatalf("expected canonical kind from PRERun, ok=%v kind=%q", ok, k)
	}
	if k != "backlog_item" {
		t.Fatalf("canonical kind = %q, want backlog_item", k)
	}
}

func TestValidateAnnotatedKind_CountAndFlags(t *testing.T) {
	// CountArg0 single kind
	cmdCount := pkgcli.NewCommandBuilder("count").Build()
	EnsureCmdAnnotations(cmdCount)
	cmdCount.Annotations[AnnotationKeyObjectKindValidate] = KindValidateCountArg0
	if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmdCount, []string{"goal"}, nil); err != nil {
		t.Fatalf("count validate failed: %v", err)
	}
	kindsSingle, ok := KindsListFromPRERun(KindAnnotKeysObject, cmdCount)
	if !ok || len(kindsSingle) != 1 || kindsSingle[0] != "goal" {
		t.Errorf("expected goal from list, got %v (ok=%v)", kindsSingle, ok)
	}

	// CountArg0 comma-separated
	cmdComma := pkgcli.NewCommandBuilder("count").Build()
	EnsureCmdAnnotations(cmdComma)
	cmdComma.Annotations[AnnotationKeyObjectKindValidate] = KindValidateCountArg0
	if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmdComma, []string{"goal,milestone"}, nil); err != nil {
		t.Fatalf("count comma validate failed: %v", err)
	}
	kinds, ok := KindsListFromPRERun(KindAnnotKeysObject, cmdComma)
	if !ok || len(kinds) != 2 {
		t.Errorf("expected 2 kinds from list, got %v (ok=%v)", kinds, ok)
	}

	// FlagKind
	cmdFlag := pkgcli.NewCommandBuilder("flag").Build()
	cmdFlag.Flags().String("kind", "goal", "")
	EnsureCmdAnnotations(cmdFlag)
	cmdFlag.Annotations[AnnotationKeyObjectKindValidate] = KindValidateFlagKind
	if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmdFlag, nil, nil); err != nil {
		t.Fatalf("flag kind validate failed: %v", err)
	}
	if k, ok := KindCanonicalFromPRERun(KindAnnotKeysObject, cmdFlag); !ok || k != "goal" {
		t.Errorf("expected goal from flag, got %s", k)
	}

	// UpdateAllKindFlag
	cmdUp := pkgcli.NewCommandBuilder("update-all").Build()
	cmdUp.Flags().Bool("all", true, "")
	cmdUp.Flags().String("kind", "milestone", "")
	EnsureCmdAnnotations(cmdUp)
	cmdUp.Annotations[AnnotationKeyObjectKindValidate] = KindValidateUpdateAllKindFlag
	if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmdUp, nil, nil); err != nil {
		t.Fatalf("update-all validate failed: %v", err)
	}
	if k, ok := KindCanonicalFromPRERun(KindAnnotKeysObject, cmdUp); !ok || k != "milestone" {
		t.Errorf("expected milestone from flag, got %s", k)
	}
}

func TestValidateAnnotatedKind_InternalListAndOptional(t *testing.T) {
	cmdList := pkgcli.NewCommandBuilder("list").Build()
	EnsureCmdAnnotations(cmdList)
	cmdList.Annotations[AnnotationKeyObjectKindValidate] = KindValidateInternalListOptional

	hookCalled := false
	hook := func(raw string) (string, bool) {
		hookCalled = true
		return "goal", true
	}

	if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmdList, []string{"goal"}, hook); err != nil {
		t.Fatalf("list validate failed: %v", err)
	}
	if !hookCalled {
		t.Errorf("expected hook to be called")
	}
	if k, ok := KindCanonicalFromPRERun(KindAnnotKeysObject, cmdList); !ok || k != "goal" {
		t.Errorf("expected goal from list prerun, got %s", k)
	}
}

func TestValidateBulkUpdateFileKind_WithFileFlag(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("bulk-update").Build()
	cmd.Flags().String("file", "updates.yaml", "")
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKeyObjectKindValidate] = KindValidateBulkUpdateFileKind

	if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, []string{"goal"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if k, ok := KindCanonicalFromPRERun(KindAnnotKeysObject, cmd); !ok || k != "goal" {
		t.Errorf("expected goal as canonical kind, got %s", k)
	}
}

func TestValidateSystemCompactStream_WithoutAllFlag(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("compact-stream-state").Build()
	cmd.Flags().Bool("all", false, "")
	cmd.Flags().String("kind", "priority_plan", "")
	EnsureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKeySystemKindValidate] = KindValidateSystemCompactStream

	if err := ValidateAnnotatedKind(KindAnnotKeysSystem, cmd, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if k, ok := KindCanonicalFromPRERun(KindAnnotKeysSystem, cmd); !ok || k != "priority_plan" {
		t.Errorf("expected priority_plan as canonical kind, got %s", k)
	}
}
