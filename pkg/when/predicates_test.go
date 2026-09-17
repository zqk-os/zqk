package when

import "testing"

const (
	testRuneComma     = ','
	testRuneColon     = ':'
	testRuneSemicolon = ';'
	testRuneDash      = '-'
	testRuneDecimal   = '.'
	testRuneNewline   = '\n'
	testRuneLetterX   = 'x'
)

func TestPredicates_StringAndNilHelpers(t *testing.T) {
	if !IsEmpty("") {
		t.Fatalf("expected empty string to be empty")
	}
	if IsEmpty("x") {
		t.Fatalf("expected non-empty string to not be empty")
	}
	if !IsBlank(" \n\t ") {
		t.Fatalf("expected whitespace-only string to be blank")
	}
	if IsBlank("x") {
		t.Fatalf("expected content string to not be blank")
	}

	var p *int
	if !IsNil(p) {
		t.Fatalf("expected nil pointer to be nil")
	}
	v := 1
	p = &v
	if !IsNotNil(p) {
		t.Fatalf("expected pointer to be non-nil")
	}
}

func TestPredicates_IsNilOrEmpty(t *testing.T) {
	var sPtr *string
	if !IsNilOrEmpty(sPtr) {
		t.Fatalf("expected nil *string to be nil/empty")
	}
	if !IsNilOrEmpty("") {
		t.Fatalf("expected empty string to be nil/empty")
	}
	if IsNilOrEmpty("x") {
		t.Fatalf("expected non-empty string to not be nil/empty")
	}
	var xs []int
	if !IsNilOrEmpty(xs) {
		t.Fatalf("expected nil slice to be nil/empty")
	}
	xs = []int{1}
	if IsNilOrEmpty(xs) {
		t.Fatalf("expected non-empty slice to not be nil/empty")
	}
}

func TestPredicates_RuneHelpers(t *testing.T) {
	if !IsComma(testRuneComma) || IsComma(testRuneSemicolon) {
		t.Fatalf("IsComma helper failed")
	}
	if !IsColon(testRuneColon) || IsColon(testRuneComma) {
		t.Fatalf("IsColon helper failed")
	}
	if !IsDash(testRuneDash) || !IsHyphen(testRuneDash) {
		t.Fatalf("dash/hyphen helpers failed")
	}
	if !IsDecimal(testRuneDecimal) || IsDecimal(testRuneComma) {
		t.Fatalf("IsDecimal helper failed")
	}
	if !IsNewLine(testRuneNewline) || IsNewLine(testRuneLetterX) {
		t.Fatalf("IsNewLine helper failed")
	}
}
