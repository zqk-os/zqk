package brand

import "strings"

const (
	defaultEnvExecutableName = "zqk"
	DefaultEnvPrefix         = "ZQK"
	emptyValue               = ""
	underscore               = "_"
	underscoreByte           = '_'
	lowerA                   = 'a'
	lowerZ                   = 'z'
	upperA                   = 'A'
	upperZ                   = 'Z'
	digit0                   = '0'
	digit9                   = '9'
)

// EnvPrefix returns the environment variable prefix derived from the executable name.
//
// Example:
//
//	ExecutableName() = "zqk"      -> EnvPrefix() = "ZQK"
//	ExecutableName() = "acme-cli" -> EnvPrefix() = "ACME_CLI"
func EnvPrefix() string {
	name := ExecutableName()
	if name == emptyValue {
		name = defaultEnvExecutableName
	}

	var b strings.Builder
	b.Grow(len(name))

	lastUnderscore := false
	for _, r := range name {
		// Upper-case ASCII letters.
		if r >= lowerA && r <= lowerZ {
			r = r - lowerA + upperA
		}

		isAZ := r >= upperA && r <= upperZ
		is09 := r >= digit0 && r <= digit9
		if isAZ || is09 {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}

		if !lastUnderscore {
			b.WriteByte(underscoreByte)
			lastUnderscore = true
		}
	}

	out := strings.Trim(b.String(), underscore)
	if out == emptyValue {
		return DefaultEnvPrefix
	}
	return out
}

// EnvVar returns a fully-qualified environment variable key using the current env prefix.
//
// Example:
//
//	EnvVar("GRAPH_ENABLED") -> "ZQK_GRAPH_ENABLED" (by default)
func EnvVar(suffix string) string {
	suffix = strings.TrimPrefix(suffix, underscore)
	return EnvPrefix() + underscore + suffix
}
