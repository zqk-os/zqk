package cli

import (
	"testing"
)

func TestStandardColorPrinters(t *testing.T) {
	cyan, green, yellow := StandardColorPrinters()
	if cyan == nil || green == nil || yellow == nil {
		t.Fatalf("expected non-nil color printer functions")
	}
	c := cyan("cyan-text")
	g := green("green-text")
	y := yellow("yellow-text")
	if len(c) == 0 || len(g) == 0 || len(y) == 0 {
		t.Errorf("expected non-empty formatted strings")
	}
}

func TestStandardUIPalette(t *testing.T) {
	palette := StandardUIPalette()
	if palette.Cyan == nil || palette.Green == nil || palette.Yellow == nil || palette.Bold == nil {
		t.Fatalf("expected non-nil UIPalette functions")
	}
	c := palette.Cyan("cyan-msg")
	g := palette.Green("green-msg")
	y := palette.Yellow("yellow-msg")
	b := palette.Bold("bold-msg")
	if len(c) == 0 || len(g) == 0 || len(y) == 0 || len(b) == 0 {
		t.Errorf("expected non-empty palette outputs")
	}
}
