# ZQK CLI Effective Use & Object Selection

ZQK relies heavily on its CLI for all knowledge graph mutations. Direct YAML editing is strictly prohibited.

## CLI Basics
*   **Initialize a system:** `zqk system init --project-name <name>`
*   **Query Objects:** `zqk object list <kind> --format table`
*   **Create/Mint Objects:** `zqk new object <kind> --title "<title>"` (mints to draft plane)
*   **Update Objects:** `zqk object update <id> --file <file.yaml>` (or `--field <key>=<val>`)
*   **Promote Objects:** `zqk object promote <id>` (advances lifecycle state)

## Object Selection: `q` vs `p`
When operating ZQK, choosing the right object scale is critical for graph hygiene.

### 1. The `q` Object (question)
*   **Scale:** Micro
*   **When to use:** Use this for quick knowledge capture, clarification, or onboarding tracking. 
*   **Example:** "How do I configure the Neo4j endpoint?" - This is a temporary ad-hoc technical block that does not warrant a full architectural milestone.

### 2. The `p` Object (priority_plan / policy)
*   **Scale:** Macro
*   **When to use:** Use this for organizing large architectural efforts and execution sequences (`PRI-*`). 
*   **Example:** "Architect Federated Capacity Advertisement". Use priority plans to group cross-cutting milestones, establish strategic objectives, and enforce project workflows across multiple backlog items.
