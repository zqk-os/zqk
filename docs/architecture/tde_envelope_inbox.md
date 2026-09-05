# Autonomy Inbox & TDE Envelopes

**Last Verified:** 2026-08-31


**Status**: Active
**Requirement**: [REDACTED-ID]

## Architecture
The Autonomy Inbox is the Human-in-the-Loop (HITL) gateway for the Symbiotic Mesh. When an agent formulates an intent that requires high-risk capabilities (e.g., mutating the filesystem or kernel state), it does not execute immediately. Instead, it packages the intent into a Time-Delayed Execution (TDE) Envelope and submits it to the Inbox.

### Core Concepts
- **TDE Envelope**: A cryptographic package containing the agent's intent, the required capability references, and the execution payload.
- **Envelope Status**: Pending, Approved, Rejected, Executed, Failed.
- **Inbox**: The registry where envelopes wait for user/policy approval.

### Interfaces
```go
package inbox

type EnvelopeStatus string

const (
	StatusPending  EnvelopeStatus = "pending"
	StatusApproved EnvelopeStatus = "approved"
	StatusRejected EnvelopeStatus = "rejected"
)

type TDEEnvelope struct {
	ID             string
	AgentID        string
	Intent         string
	CapabilityRefs []string
	Payload        []byte
	Status         EnvelopeStatus
}

type Inbox interface {
	Submit(env TDEEnvelope) error
	ListPending() []TDEEnvelope
	Approve(id string) error
	Reject(id string, reason string) error
}
```

### Multi-Agent Orchestration (Convergence Sessions)
The Autonomy Inbox is structurally designed to support complex, multi-agent orchestrations (e.g., during a `convergence_session`). 
When multiple agents collaborate on a complex goal:
1. **Delegation & Staging**: Delegated agents do not execute destructive actions directly. Instead, they stage TDE Envelopes in the Autonomy Inbox.
2. **Dependency Chains**: Agents encode the order of operations using the `DependsOn` field. For example, a `build` envelope can depend on a `code_generation` envelope.
3. **Approval Flow**: The Lead Orchestrator, Trust Sentinel, or Human-in-the-Loop acting as the authority can review the DAG of operations. The Inbox mechanism strictly enforces the approval order, blocking the approval of dependent envelopes until their prerequisites are fulfilled.
This provides a safe, inspectable, and repeatable synchronization pattern for AI-driven parallel tracks.

### Future Considerations
- Automatic approval by the Trust Sentinel for low-risk capabilities.
- Push notifications via MCP for pending envelopes.
