// Package agentdelivery provides pluggable transport for agent prompts (markdown and metadata)
// after they are produced—separate from prompt content and from pkg/agentprompt wire format.
//
// FileDeliverer writes atomically; HTTPDeliverer POSTs JSON; AppendDeliveryAuditJSONL records
// each delivery to .zqk/logs/scheduler/agent_prompt_deliveries.jsonl.
//
// Rationale: REQ-AO-001 / ITEM-AO-001 — delivery must not be hard-coded to Cursor or a single OS.
// See docs/architecture/AGENT_ORCHESTRATION_AUDIBLE.md and docs/architecture/SCENARIO_BUNDLES_AND_TRACEABILITY.md.

package agentdelivery
