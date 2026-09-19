package swarm

import "fmt"

const (
	// swarmListResultCap is tight because object_list/object_count are
	// context-gathering, not mutation evidence.
	swarmListResultCap = 4000
	// swarmReadResultCap still lets the model see a real file slice.
	swarmReadResultCap = 8000
	// swarmDefaultResultCap replaced the old 25KB silent trim that left a
	// 25KB dump in the transcript after a 679KB list.
	swarmDefaultResultCap = 8000
)

// clipSwarmToolResult shortens huge tool payloads and appends write-now
// guidance so a dump cannot crowd the 7B off mutation tools.
// tools enforce these caps at the executor and seats stop parking for
// matched-0 writes after a context dump.
func clipSwarmToolResult(toolName, result string) (string, bool) {
	capBytes := swarmToolResultCap(toolName)
	if len(result) <= capBytes {
		return result, false
	}
	return result[:capBytes] + "\n\n" + TruncatedToolResultGuidance(), true
}

func swarmToolResultCap(toolName string) int {
	switch {
	case toolSuffixIs(toolName, "object_list"), toolSuffixIs(toolName, "object_count"):
		return swarmListResultCap
	case toolSuffixIs(toolName, "read_code"), toolSuffixIs(toolName, "read_file"),
		toolSuffixIs(toolName, "object_get"):
		return swarmReadResultCap
	default:
		return swarmDefaultResultCap
	}
}

func TruncatedToolResultGuidance() string {
	p := DefaultToolPrefix()
	return fmt.Sprintf(
		"... [OUTPUT TRUNCATED BY KERNEL] That dump is not task evidence. "+
			"Do not list or re-read. Call %swrite_code or %swrite_file on a real cmd/ or pkg/ path now.",
		p, p,
	)
}
