package specorigination

import "testing"

func TestValidateOptions(t *testing.T) {
	t.Parallel()
	if err := ValidateOptions(Options{}); err == nil {
		t.Fatal("expected error for empty options")
	}
	if err := ValidateOptions(Options{ProjectRoot: "/tmp", Ontology: ""}); err == nil {
		t.Fatal("expected error for empty ontology")
	}
	if err := ValidateOptions(Options{ProjectRoot: "", Ontology: "x"}); err == nil {
		t.Fatal("expected error for empty project root")
	}
	if err := ValidateOptions(Options{ProjectRoot: "/tmp", Ontology: "Bad"}); err == nil {
		t.Fatal("expected error for invalid ontology stem")
	}
	if err := ValidateOptions(Options{ProjectRoot: "/tmp", Ontology: "ok_kind"}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}
