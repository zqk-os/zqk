package agentfeed

import (
	"regexp"
	"strings"
)

// TRACK: BLI-COMMS-ORCH-EXECUTE-NOT-ACK-001 — hourglass bodies name one ATK plus a
// "do not execute" sibling; only the ONLY-marked id (or a unique id) is live.
var (
	namedATKIDPattern = regexp.MustCompile(`\bATK-[0-9]+-[0-9a-fA-F]+\b`)
	onlyATKIDPattern  = regexp.MustCompile(`(?i)\bONLY\s+(ATK-[0-9]+-[0-9a-fA-F]+)\b`)
)

// NamedATKID returns the single ATK a directed steer is asking the seat to execute.
// Prefer "ONLY ATK-…". If that is absent, accept exactly one ATK id. Two or more
// unmarked ids (leftover pile) return empty so the worker refuses.
func NamedATKID(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if m := onlyATKIDPattern.FindStringSubmatch(body); len(m) == 2 {
		return m[1]
	}
	found := namedATKIDPattern.FindAllString(body, 3)
	if len(found) != 1 {
		return ""
	}
	return found[0]
}
