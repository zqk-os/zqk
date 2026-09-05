package when

import (
	"reflect"
	"strings"
	"unicode"
)

const (
	emptyString    = ""
	runeNewLine    = '\n'
	runeCarriage   = '\r'
	runeComma      = ','
	runeColon      = ':'
	runeDash       = '-'
	runeTilde      = '~'
	runeFwdSlash   = '/'
	runeBackSlash  = '\\'
	runeLeftCurly  = '{'
	runeRightCurly = '}'
	runeDecimal    = '.'
)

// IsEmpty reports whether s is empty.
func IsEmpty(s string) bool {
	return s == emptyString
}

// IsBlank reports whether s is empty or whitespace-only.
func IsBlank(s string) bool {
	return strings.TrimSpace(s) == emptyString
}

// IsNil reports whether v is nil.
func IsNil[T any](v *T) bool {
	return v == nil
}

// IsNotNil reports whether v is non-nil.
func IsNotNil[T any](v *T) bool {
	return v != nil
}

// IsNilValue reports whether v is nil, including typed nil values in interfaces.
func IsNilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// IsNilOrEmpty reports whether v is nil or empty.
// Supported empty cases: string, pointer-to-string, arrays, slices, maps.
func IsNilOrEmpty(v any) bool {
	if IsNilValue(v) {
		return true
	}
	switch x := v.(type) {
	case string:
		return x == emptyString
	case *string:
		return x == nil || *x == emptyString
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Array, reflect.Slice, reflect.Map, reflect.String:
		return rv.Len() == 0
	default:
		return false
	}
}

func IsNewLine(r rune) bool    { return r == runeNewLine || r == runeCarriage }
func IsComma(r rune) bool      { return r == runeComma }
func IsColon(r rune) bool      { return r == runeColon }
func IsDash(r rune) bool       { return r == runeDash }
func IsHyphen(r rune) bool     { return r == runeDash }
func IsTilde(r rune) bool      { return r == runeTilde }
func IsFwdSlash(r rune) bool   { return r == runeFwdSlash }
func IsBackSlash(r rune) bool  { return r == runeBackSlash }
func IsLeftCurly(r rune) bool  { return r == runeLeftCurly }
func IsRightCurly(r rune) bool { return r == runeRightCurly }
func IsDecimal(r rune) bool    { return r == runeDecimal }
func IsChar(r rune) bool       { return unicode.IsLetter(r) }
func IsNum(r rune) bool        { return unicode.IsDigit(r) }
