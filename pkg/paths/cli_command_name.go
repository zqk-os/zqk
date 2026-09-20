package paths

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
)

// CLICommandNameDefault is the default executable name for the CLI.
// It can be overridden at runtime during initialization to support white-labeling.
const CLICommandNameDefault = "zqk"

// CLICommandName is the executable/command name used in help text, examples, and suggestions.
// It is set during CLI initialization from config (brand.executable_name) or the actual binary name.
var CLICommandName = CLICommandNameDefault

// CLIName is the live product executable token for usage, hints, and examples.
func CLIName() string {
	if n := strings.TrimSpace(brand.ExecutableName()); n != "" {
		return n
	}
	if n := strings.TrimSpace(CLICommandName); n != "" {
		return n
	}
	return CLICommandNameDefault
}

// CLIUsage joins the live executable name with subcommand tokens.
func CLIUsage(args ...string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, CLIName())
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a != "" {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

// CLIInvocation rewrites a user-facing invocation to the live executable name.
// Lines that are not product subcommands (git, go test, …) are returned unchanged.
// Compound lines joined with && are rewritten per segment.
func CLIInvocation(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return line
	}
	if strings.Contains(line, "&&") {
		parts := strings.Split(line, "&&")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			out = append(out, cliInvocationOne(strings.TrimSpace(p)))
		}
		return strings.Join(out, " && ")
	}
	return cliInvocationOne(line)
}

func cliInvocationOne(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return line
	}
	line = stripCLIPrefix(line)
	if !looksLikeProductSubcommand(line) {
		return line
	}
	return CLIName() + " " + line
}

// IsProductCLIVerb reports whether token is a top-level product command.
func IsProductCLIVerb(verb string) bool {
	_, ok := productCLIVerbs[strings.TrimSpace(verb)]
	return ok
}

// RewriteCanonicalCLIInvocations replaces authored "zqk <verb>" (and the live
// executable name) in prose so hints stay brand-portable.
func RewriteCanonicalCLIInvocations(s string) string {
	if s == "" {
		return s
	}
	live := CLIName()
	canon := brand.CanonicalExecutableToken
	out := s
	if canon != "" && canon != live {
		out = rewriteCLIToken(out, canon, live)
	}
	if live != "" {
		out = rewriteCLIToken(out, live, live)
	}
	return out
}

func stripCLIPrefix(line string) string {
	for _, prefix := range []string{CLIName() + " ", brand.CanonicalExecutableToken + " "} {
		if prefix != " " && strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	return line
}

func looksLikeProductSubcommand(line string) bool {
	verb, _, _ := strings.Cut(strings.TrimSpace(line), " ")
	_, ok := productCLIVerbs[verb]
	return ok
}

func rewriteCLIToken(s, from, to string) string {
	if from == "" || to == "" {
		return s
	}
	needle := from + " "
	if !strings.Contains(s, needle) {
		return s
	}
	var b strings.Builder
	rest := s
	for {
		i := strings.Index(rest, needle)
		if i < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:i])
		after := rest[i+len(needle):]
		verb, _, _ := strings.Cut(after, " ")
		if _, ok := productCLIVerbs[verb]; ok {
			b.WriteString(to)
			b.WriteByte(' ')
		} else {
			b.WriteString(needle)
		}
		rest = after
	}
}

var productCLIVerbs = map[string]struct{}{
	"agent": {}, "ambient": {}, "auth": {}, "automation": {}, "callback": {},
	"ci": {}, "completion": {}, "convergence": {}, "docman": {}, "domain": {},
	"feed": {}, "graph": {}, "grep": {}, "healthchk": {}, "inbox": {},
	"intake": {}, "internal": {}, "job": {}, "join": {}, "keystore": {},
	"learn": {}, "matrix": {}, "mcp": {}, "mesh": {}, "new": {}, "object": {},
	"observer": {}, "ontology": {}, "ops": {}, "organizational": {}, "pplan": {},
	"pre-commit": {}, "quickstart": {}, "reports": {}, "rollback": {},
	"scheduler": {}, "semantic": {}, "service": {}, "spec": {}, "swarm": {},
	"system": {}, "test": {}, "tray": {}, "use": {}, "utility": {},
	"validate": {}, "vendor": {}, "version": {}, "workflow": {},
}
