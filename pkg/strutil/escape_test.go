package strutil

import "testing"

func TestDecodeEscapeRune(t *testing.T) {
	tests := []struct {
		in   rune
		want rune
	}{
		{'n', '\n'},
		{'t', '\t'},
		{'r', '\r'},
		{'\\', '\\'},
		{'"', '"'},
		{'\'', '\''},
		{'x', 'x'},
	}

	for _, tt := range tests {
		if got := DecodeEscapeRune(tt.in); got != tt.want {
			t.Errorf("DecodeEscapeRune(%c) = %c; want %c", tt.in, got, tt.want)
		}
	}
}
