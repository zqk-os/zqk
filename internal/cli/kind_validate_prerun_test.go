package cli

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

func TestValidateAnnotatedKind_noAnnotation_isNoOp(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "x"}
	if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, []string{"any"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBulkUpdateFileKind_skipsWithoutFileFlag(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "bulk-update"}
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
	cmd := &cobra.Command{Use: "compact-stream-state"}
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
	cmd := &cobra.Command{Use: "fields"}
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
