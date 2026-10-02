package strutil

// DecodeEscapeRune converts an escaped rune (e.g. 'n', 't') into its unescaped character.
func DecodeEscapeRune(escaped rune) rune {
	switch escaped {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	case '\\':
		return '\\'
	case '"':
		return '"'
	case '\'':
		return '\''
	default:
		return escaped
	}
}
