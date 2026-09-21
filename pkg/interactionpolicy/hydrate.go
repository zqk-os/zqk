package interactionpolicy

import (
	"strings"
	"unicode"

	"github.com/zqk-os/zqk/pkg/objects"
)

// maxDriveRunes caps the compiled hunger signal so hooks stay short.
const maxDriveRunes = 600

// SkipCASOverlay is true when catalog text must win over the shared POL body.
// Idle, push-ahead, and inbox share PROCESS-ADMIN; overlay would replace them
// with empty-column or "ack then hourglass" prose. TRACK
func SkipCASOverlay(event string) bool {
	switch event {
	case EventIdle, EventPushAhead, EventInboxUnacked:
		return true
	default:
		return false
	}
}

// OverlayFromPolicy replaces catalog GuidingStep with drive extracted from a
// CAS policy body. Returns false when the body yields nothing useful — keep
// the catalog reflex. TRACK
func OverlayFromPolicy(step *Step, policy map[string]any) bool {
	if step == nil || policy == nil {
		return false
	}
	body, _ := policy[objects.FieldKeyBody].(string)
	drive := DriveFromPolicyBody(body)
	if drive == "" {
		return false
	}
	step.GuidingStep = drive
	return true
}

// DriveFromPolicyBody compiles policy markdown into one guiding paragraph.
// Prefers a Response section (the gland); otherwise the first prose block.
func DriveFromPolicyBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	section := extractResponseSection(body)
	if section == "" {
		section = firstProseParagraph(body)
	}
	section = collapseSpace(section)
	if section == "" {
		return ""
	}
	return clipRunes(section, maxDriveRunes)
}

func extractResponseSection(body string) string {
	lower := strings.ToLower(body)
	idx := strings.Index(lower, "**response")
	if idx < 0 {
		idx = strings.Index(lower, "\n## response")
	}
	if idx < 0 && strings.HasPrefix(lower, "## response") {
		idx = 0
	}
	if idx < 0 {
		return ""
	}
	rest := body[idx:]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:]
	} else {
		return ""
	}
	if cut := strings.Index(rest, "\n## "); cut >= 0 {
		rest = rest[:cut]
	}
	// POL bodies use **Parent:** / **Trigger:** as the next field, not ##.
	lowerRest := strings.ToLower(rest)
	for _, marker := range []string{"\n**parent", "\n**trigger", "\n**persona"} {
		if cut := strings.Index(lowerRest, marker); cut >= 0 {
			rest = rest[:cut]
			lowerRest = lowerRest[:cut]
		}
	}
	return strings.TrimSpace(rest)
}

func firstProseParagraph(body string) string {
	var b strings.Builder
	for _, line := range strings.Split(body, "\n") {
		s := strings.TrimSpace(line)
		if s == "" {
			if b.Len() > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(s, "#") {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s)
	}
	return strings.TrimSpace(b.String())
}

func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func clipRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	n := 0
	for i := range s {
		if n == max {
			return strings.TrimSpace(s[:i]) + "…"
		}
		n++
	}
	return s
}
