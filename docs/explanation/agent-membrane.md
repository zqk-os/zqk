# Explanation: The Agent Admin Membrane Pattern

How ZQK separates cognitive coding from administrative, lifecycle, and filesystem operations.

---

## 1. The Threat of Procedural Hallucination
When LLMs are given unbounded authority to create branches, modify configuration, and touch database schemas, small hallucinations compound into systemic corruption.

---

## 2. The Membrane Separation
The Agent Admin Membrane splits responsibility into two distinct layers:
1. **The Cognitive Worker (Inside the Membrane):**
   - Focuses strictly on code drafting, test creation, and bug fixing.
   - Operates in an isolated worktree branch with read-only knowledge tools and write-only code tools.
   - Cannot directly mutate process YAML or alter lifecycle states.
2. **The Seat Worker / Supervisor (Outside the Membrane):**
   - Monitors worktree output and test outcomes.
   - Evaluates gates, stamps commit hashes, and executes lifecycle promotions (`object promote`).
   - Merges verified branches into integration streams.
