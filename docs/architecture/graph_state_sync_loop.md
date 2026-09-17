# Graph-State Sync Loop Architecture

## Overview
The "ReAct + MCP/Chat History" pattern is fundamentally broken for a Knowledge Operating System like ZQK because it conflates **working memory** (the conversation context) with **knowledge state** (the graph). 

This architecture outlines the **Graph-State Sync Loop**, replacing the prompt history loop entirely. The LLM becomes a purely functional node in the graph that consumes context and produces state transitions.

## 1. Core Orchestrator Loop (`cmd/zqk/agent/sync_loop.go`)
Replaces the ReAct/MCP history loop with a scheduler-driven worker pool. The LLM never sees conversation history; it only sees a bounded task snapshot and its resolved dependencies.

```go
func RunGraphOrchestrator(ctx context.Context, taskID string, g graph.Graph, llm ModelClient) error {
	const (
		maxPollDuration = 15 * time.Minute
		pollInterval    = 2 * time.Second
		contextDepth    = 2
	)

	ctx, cancel := context.WithTimeout(ctx, maxPollDuration)
	defer cancel()

	poller := time.NewTicker(pollInterval)
	defer poller.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("orchestrator timeout")
		case <-poller.C:
			// 1. FRESH STATE READ EVERY TICK
			currentTask, err := g.GetNode(taskID)
			if err != nil { return err }

			// EXIT CONDITION
			if currentTask.Status == "implemented" || currentTask.Status == "error" {
				return nil
			}

			// 2. Resolve Bounded Context via QuerySubgraph
			deps, edges, err := g.QuerySubgraph(taskID, contextDepth)
			if err != nil { return fmt.Errorf("subgraph resolution failed: %w", err) }

			contextBundle := buildContextBundle(currentTask, deps, edges)
			prompt := constructStatelessPrompt(currentTask, contextBundle)

			// 3. Invoke LLM with Strict Schema Contract
			resp, err := llm.Call(ctx, CallRequest{
				ModelID: currentTask.Assignee,
				Schema:  mutation.OutputSchema(),
				Prompt:  prompt,
			})
			if err != nil { return fmt.Errorf("model call failed: %w", err) }

			// 4. Validate & Map to Mutation
			var rawOut map[string]any
			json.Unmarshal([]byte(resp.Content), &rawOut)
			mut, _ := mutation.FromMap(rawOut)
			
			// 5. Validation & Idempotency Key
			ik := mutation.BuildIDKey(taskID, mut)
			if exists, _ := g.HasCommittedMutation(ctx, ik); exists {
				continue
			}

			// 6. Audit Hook PreFlight
			audHook.PreFlight(ctx, mut, ik, resp.Meta.ID, taskID)

			// 7. HIL Gate Safety Check
			if mut.SafetyClass == "destructive" || mut.SafetyClass == "hil_required" {
				if err := g.HilGate(ctx, taskID, mut); err != nil {
					g.UpdateStatus(taskID, "blocked")
					continue
				}
			}

			// 8. Commit Mutation to Graph Kernel
			if err := g.CommitMutation(ik, []mutation.Mutation{mut}); err != nil {
				return err
			}

			audHook.MarkApplied(ctx, ik, mut.ResultHashHint())
			if mut.StatusTransition != "" { 
				g.UpdateStatus(taskID, mut.StatusTransition) 
			}
		}
	}
}
```

## 2. Validation & Safety Gate Logic
Intercepts unsafe mutations before they touch the graph kernel. Uses a promise-based HIL gate that never blocks the scheduler loop.

```go
var Schema = `{
  "type": "object",
  "required": ["action", "target_kind"],
  "properties": {
    "action": {"enum": ["create_node", "update_node", "add_edge", "remove_edge"]},
    "target_kind": {"type": "string"},
    "target_id": {"type": "string"},
    "fields": {"type": "object"},
    "edges": {"type": "array", "items": {"$ref": "#/definitions/EdgeMutation"}},
    "status_transition": {"type": "string"},
    "safety_class": {"enum": ["read", "write", "destructive", "hil_required"]}
  }
}`
```

## 3. Idempotency Key Strategy
Ensures crash recovery never applies duplicate mutations. Keys are computed before HIL/audit, so retries reuse the exact same key. Maps are sorted deterministically before hashing.

```go
func BuildIDKey(taskID string, mut *Mutation) string {
	// 1. Normalize fields (maps are unordered in Go; must sort keys & stringify values)
	// 2. Normalize edges (sort by relation+target_id)
	// 3. Hash normalized payload
	data, _ := json.Marshal(payload)
	return "ik::" + fmt.Sprintf("%x", sha256.Sum256(data))
}
```

## 4. Enterprise Audit Hook
Append-only JSONL WAL that survives process crashes. Logs intent before execution, enabling full audit trails and drift detection.
