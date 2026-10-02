package cli

import "testing"

func TestStandardColorPrinters(t *testing.T) {
	cyan, green, yellow := StandardColorPrinters()
	if cyan == nil || green == nil || yellow == nil {
		t.Fatal("expected non-nil color printers")
	}
	if cyan("test") == "" || green("test") == "" || yellow("test") == "" {
		t.Fatal("expected non-empty formatted output")
	}
}
