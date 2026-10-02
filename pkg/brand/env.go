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
	return EnvPrefixForExecutable(ExecutableName())
}

// EnvPrefixForExecutable returns the environment variable prefix for a named binary
// without changing the process-wide ExecutableName.
// Channel suffixes (-stable, -community, -dev, …) are stripped so install aliases
// like zqk-stable keep reading ZQK_* (not ZQK_STABLE_*).
func EnvPrefixForExecutable(name string) string {
	if name == emptyValue {
		name = defaultEnvExecutableName
	}
	name = stripEnvChannelSuffix(name)

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

// stripEnvChannelSuffix removes packaging/channel suffixes from an executable basename
// before deriving the env prefix. Order: longest / most specific first.
func stripEnvChannelSuffix(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, suffix := range []string{
		".test",
		"-mcp-ide-adapter", "-mcp-proxy", "-mcp-daemon", "-mcp",
		"-privileged-writer", "-pw",
		"-ambient-daemon", "-ambient", "-amb",
		"-scheduler-daemon", "-scheduler", "-sched",
		"-overseer",
		"-community", "-stable", "-dev", "-beta", "-alpha", "-rc",
	} {
		if strings.HasSuffix(lower, suffix) {
			trimmed := strings.TrimSuffix(lower, suffix)
			if trimmed != emptyValue {
				return trimmed
			}
		}
	}
	return name
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
