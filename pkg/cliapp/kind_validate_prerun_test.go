package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

func TestResolveKindFromPositionalAndFlag(t *testing.T) {
	t.Parallel()
	root := "."

	tests := []struct {
		name      string
		pos       string
		flag      string
		wantKind  string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "neither provided",
			pos:      "",
			flag:     "",
			wantKind: "",
			wantErr:  false,
		},
		{
			name:     "positional only",
			pos:      "backlog_item",
			flag:     "",
			wantKind: "backlog_item",
			wantErr:  false,
		},
		{
			name:     "flag only",
			pos:      "",
			flag:     "requirement",
			wantKind: "requirement",
			wantErr:  false,
		},
		{
			name:     "matching both identical",
			pos:      "backlog_item",
			flag:     "backlog_item",
			wantKind: "backlog_item",
			wantErr:  false,
		},
		{
			name:     "matching both via synonym alias",
			pos:      "req",
			flag:     "requirement",
			wantKind: "requirement",
			wantErr:  false,
		},
		{
			name:      "contradicting kinds",
			pos:       "requirement",
			flag:      "epic",
			wantErr:   true,
			errSubstr: "contradicts",
		},
		{
			name:      "invalid positional kind",
			pos:       "nonexistent_kind_xyz",
			flag:      "",
			wantErr:   true,
			errSubstr: "unknown kind",
		},
		{
			name:      "invalid flag kind",
			pos:       "",
			flag:      "nonexistent_kind_xyz",
			wantErr:   true,
			errSubstr: "unknown kind",
		},
		{
			name:      "valid positional but invalid flag",
			pos:       "requirement",
			flag:      "nonexistent_kind_xyz",
			wantErr:   true,
			errSubstr: "unknown kind",
		},
		{
			name:      "invalid positional but valid flag",
			pos:       "nonexistent_kind_xyz",
			flag:      "requirement",
			wantErr:   true,
			errSubstr: "unknown kind",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveKindFromPositionalAndFlag(root, tt.pos, tt.flag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errSubstr)
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tt.wantKind {
					t.Fatalf("got kind %q, want %q", got, tt.wantKind)
				}
			}
		})
	}
}

func TestValidateAnnotatedKind_Positional0Opt_Permutations(t *testing.T) {
	t.Parallel()

	buildCmd := func(flagVal string) *cobra.Command {
		cmd := pkgcli.NewCommandBuilder("list").Build()
		cmd.Flags().String("kind", "", "")
		if flagVal != "" {
			_ = cmd.Flags().Set("kind", flagVal)
		}
		EnsureCmdAnnotations(cmd)
		cmd.Annotations[AnnotationKeyObjectKindValidate] = KindValidatePositional0Opt
		return cmd
	}

	t.Run("neither flag nor positional", func(t *testing.T) {
		cmd := buildCmd("")
		if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := KindCanonicalFromPRERun(KindAnnotKeysObject, cmd); ok {
			t.Fatal("expected no canonical kind when neither positional nor flag is set")
		}
	})

	t.Run("flag only", func(t *testing.T) {
		cmd := buildCmd("requirement")
		if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		k, ok := KindCanonicalFromPRERun(KindAnnotKeysObject, cmd)
		if !ok || k != "requirement" {
			t.Fatalf("expected requirement, got %q (ok=%v)", k, ok)
		}
	})

	t.Run("positional only", func(t *testing.T) {
		cmd := buildCmd("")
		if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, []string{"backlog_item"}, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		k, ok := KindCanonicalFromPRERun(KindAnnotKeysObject, cmd)
		if !ok || k != "backlog_item" {
			t.Fatalf("expected backlog_item, got %q (ok=%v)", k, ok)
		}
	})

	t.Run("both matching", func(t *testing.T) {
		cmd := buildCmd("backlog_item")
		if err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, []string{"backlog_item"}, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		k, ok := KindCanonicalFromPRERun(KindAnnotKeysObject, cmd)
		if !ok || k != "backlog_item" {
			t.Fatalf("expected backlog_item, got %q (ok=%v)", k, ok)
		}
	})

	t.Run("both contradicting", func(t *testing.T) {
		cmd := buildCmd("epic")
		err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, []string{"requirement"}, nil)
		if err == nil {
			t.Fatal("expected error when positional and flag contradict, got nil")
		}
		if !strings.Contains(err.Error(), "contradicts") {
			t.Fatalf("expected 'contradicts' in error, got: %v", err)
		}
	})

	t.Run("invalid kind in flag", func(t *testing.T) {
		cmd := buildCmd("invalid_kind_xyz")
		err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, nil, nil)
		if err == nil {
			t.Fatal("expected error for invalid flag kind, got nil")
		}
	})

	t.Run("invalid kind in positional", func(t *testing.T) {
		cmd := buildCmd("")
		err := ValidateAnnotatedKind(KindAnnotKeysObject, cmd, []string{"invalid_kind_xyz"}, nil)
		if err == nil {
			t.Fatal("expected error for invalid positional kind, got nil")
		}
	})
}
