// Package antigravity adapts Gemini Antigravity CLI "brain" delivery.
//
// Conversation layout (vendor, not kernel):
//
//	~/.gemini/antigravity-cli/brain/<conversation>/.system_generated/logs/transcript.jsonl
//	~/.gemini/antigravity-cli/brain/<conversation>/.system_generated/messages/undelivered/
//
// Kernel code must not hard-code these segments; use this package.
package antigravity
