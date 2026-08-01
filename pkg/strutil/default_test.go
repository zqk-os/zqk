package strutil

import "testing"

const (
	testValueA      = "a"
	testValueB      = "b"
	testValueC      = "c"
	testWhitespace  = " "
	testValueX      = "x"
	testDefaultD    = "d"
	testEmptyString = ""
	testIntZero     = 0
	testIntSeven    = 7
	testIntFortyTwo = 42
)

func TestOrDefault(t *testing.T) {
	tests := []struct {
		value      string
		defaultVal string
		want       string
	}{
		{testValueA, testValueB, testValueA},
		{testEmptyString, testValueB, testValueB},
		{testWhitespace, testValueB, testWhitespace},
		{testEmptyString, testEmptyString, testEmptyString},
	}
	for _, tt := range tests {
		got := OrDefault(tt.value, tt.defaultVal)
		if got != tt.want {
			t.Errorf("OrDefault(%q, %q) = %q, want %q", tt.value, tt.defaultVal, got, tt.want)
		}
	}
}

func TestDefaultWhenNonEmpty(t *testing.T) {
	tests := []struct {
		value      string
		defaultVal string
		want       string
	}{
		{testValueA, testValueB, testValueB},
		{testEmptyString, testValueB, testEmptyString},
		{testWhitespace, testValueB, testValueB},
		{testEmptyString, testEmptyString, testEmptyString},
	}
	for _, tt := range tests {
		got := DefaultWhenNonEmpty(tt.value, tt.defaultVal)
		if got != tt.want {
			t.Errorf("DefaultWhenNonEmpty(%q, %q) = %q, want %q", tt.value, tt.defaultVal, got, tt.want)
		}
	}
}

func TestPtrOrDefault(t *testing.T) {
	s := testValueX
	nilPtr := (*string)(nil)
	if got := PtrOrDefault(&s, testDefaultD); got != testValueX {
		t.Errorf("PtrOrDefault(non-nil, d) = %q, want %q", got, testValueX)
	}
	if got := PtrOrDefault(nilPtr, testDefaultD); got != testDefaultD {
		t.Errorf("PtrOrDefault(nil, d) = %q, want %q", got, testDefaultD)
	}
	i := testIntFortyTwo
	if got := PtrOrDefault(&i, testIntZero); got != testIntFortyTwo {
		t.Errorf("PtrOrDefault(non-nil int, 0) = %v, want 42", got)
	}
	if got := PtrOrDefault((*int)(nil), testIntSeven); got != testIntSeven {
		t.Errorf("PtrOrDefault(nil int, 7) = %v, want 7", got)
	}
}

func TestFirstNonNil(t *testing.T) {
	a, b, c := testValueA, testValueB, testValueC
	if got := FirstNonNil(&a, &b, &c); got == nil || *got != testValueA {
		t.Errorf("FirstNonNil(a,b,c) = %v, want &a", got)
	}
	if got := FirstNonNil((*string)(nil), &b, &c); got == nil || *got != testValueB {
		t.Errorf("FirstNonNil(nil,b,c) = %v, want &b", got)
	}
	if got := FirstNonNil((*string)(nil), (*string)(nil), &c); got == nil || *got != testValueC {
		t.Errorf("FirstNonNil(nil,nil,c) = %v, want &c", got)
	}
	if got := FirstNonNil((*string)(nil), (*string)(nil)); got != nil {
		t.Errorf("FirstNonNil(nil,nil) = %v, want nil", got)
	}
	if got := FirstNonNil[string](); got != nil {
		t.Errorf("FirstNonNil() = %v, want nil", got)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"a\r\nb\r\nc", []string{"a", "b", "c"}},
		{"a\rb\rc", []string{"a", "b", "c"}},
		{"\n\r\n\r", []string{"", "", "", ""}},
		{"", []string{""}},
	}
	for _, tt := range tests {
		got := SplitLines(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("SplitLines(%q) returned %d lines, want %d", tt.input, len(got), len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("SplitLines(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}
