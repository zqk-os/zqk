package system

const (
	synthesizeCmdUse         = "synthesize [intent_id]"
	synthesizeCmdShort       = "Synthesize a new autonomous capability"
	synthesizeCmdLong        = "The synthesize command pulls an intent submission from the object store and processes it through the Orchestrator Registry to dynamically generate a new Agent Capability."
	synthesizeCmdDescription = "The synthesize command is the entry point to the Autonomous Kernel's dynamic capability generation system. It verifies intent via Policy Engine, extracts semantic context via the Hivemind, and commits the capability."
	synthesizeHelpDesc1      = "• Requires a valid Intent object ID."
	synthesizeHelpDesc2      = "• Executes Phase 2 Synthesis Orchestration."
	synthesizeHelpExample    = "Synthesize a new capability from an intent object"
	synthesizeHelpExampleCmd = "zqk system synthesize INT-12345"

	synthesizeErrProjectRoot   = "project root is missing"
	synthesizeErrMissingArg    = "missing required argument: intent_id"
	synthesizeErrObjectStore   = "failed to initialize storage context"
	synthesizeErrInvalidIntent = "invalid intent format or object not found: %s"
	synthesizeStatusStart      = "🤖 Initializing Capability Synthesis Orchestrator...\n"
	synthesizeStatusComplete   = "✅ Capability Synthesized: %s (ID: %s)\n"
)
