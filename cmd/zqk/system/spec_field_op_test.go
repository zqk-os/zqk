package system

import "testing"

func TestResolveSpecFieldOperation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"define", FieldOpCreate},
		{"DEFINE", FieldOpCreate},
		{" add ", FieldOpCreate},
		{"create", FieldOpCreate},
		{"modify", FieldOpModify},
		{"deprecate", FieldOpDeprecate},
		{"archive", FieldOpArchive},
		{"delete", FieldOpDelete},
	}
	for _, tt := range tests {
		if got := ResolveSpecFieldOperation(tt.in); got != tt.want {
			t.Errorf("ResolveSpecFieldOperation(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFieldOpRequiresDefinitionFile(t *testing.T) {
	t.Parallel()
	if !FieldOpRequiresDefinitionFile("define") || !FieldOpRequiresDefinitionFile(FieldOpModify) {
		t.Fatal("expected true for define/modify")
	}
	if FieldOpRequiresDefinitionFile(FieldOpDeprecate) {
		t.Fatal("deprecate should not require definition file")
	}
}

func TestFieldOperation_ResolvedOperation(t *testing.T) {
	t.Parallel()
	op := FieldOperation{Operation: "define"}
	if got := op.ResolvedOperation(); got != FieldOpCreate {
		t.Fatalf("got %q", got)
	}
}
