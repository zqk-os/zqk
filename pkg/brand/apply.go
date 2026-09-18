package brand

import (
	"regexp"
	"strings"
)

// CanonicalExecutableToken is the default CLI name in source, specs, and docs.
// Pressure-test aliases (e.g. zcom) belong in brand.executable_name, not in copy.
const CanonicalExecutableToken = defaultExecutableNameValue

var (
	// Match a bare "zqk" command token. Do not rewrite:
	//   .zqk           data dir
	//   zqk.yaml       zqk-local.yaml  zqk-stable  (hyphenated / dotted filenames)
	// Go's regexp has no lookahead; consume the following delimiter and put it back.
	reCanonicalExec = regexp.MustCompile(`(^|[^.\w-])zqk($|[^.\w-])`)
	reProductExact  = regexp.MustCompile(`\b(ZQK|NEXOS)\b`)
)

// ApplyCanonicalExecutable rewrites canonical `zqk` CLI/env tokens to executable.
// Kernel paths under `.zqk/` are left intact. Empty or canonical executable is a no-op
// for the command token (env prefix still applied when it differs).
func ApplyCanonicalExecutable(s, executable string) string {
	if s == emptyBrandValue {
		return s
	}
	exe := strings.TrimSpace(executable)
	if exe == emptyBrandValue {
		exe = CanonicalExecutableToken
	}
	if pfx := EnvPrefixForExecutable(exe); pfx != DefaultEnvPrefix {
		s = strings.ReplaceAll(s, DefaultEnvPrefix+"_", pfx+"_")
	}
	if exe == CanonicalExecutableToken {
		return s
	}
	return reCanonicalExec.ReplaceAllString(s, "${1}"+exe+"${2}")
}

// ApplyProductName rewrites standalone ZQK/NEXOS product tokens.
func ApplyProductName(s, product string) string {
	if s == emptyBrandValue || strings.TrimSpace(product) == emptyBrandValue {
		return s
	}
	return reProductExact.ReplaceAllStringFunc(s, func(match string) string {
		if match == strings.ToUpper(match) {
			return strings.ToUpper(product)
		}
		return product
	})
}

// ApplyToUserText rewrites canonical CLI tokens and product name for help/docs.
func ApplyToUserText(s, executableName, productName string) string {
	s = ApplyCanonicalExecutable(s, executableName)
	s = ApplyProductName(s, productName)
	return s
}
