# Agent Context Injection and Semantic Filtering Architecture

**Last Verified:** 2026-08-31


**Objective:** Prevent agentic context explosion, eliminate semantic collision (hallucinations), and strictly govern agent autonomy by binding creativity to designated phases and domains. We avoid building "spherical cows" by grounding implementation agents in pedantic reality, while allowing unbridled creativity during design inception.

## 1. The Tripartite Agent Identity
Every agent operating in ZQK MUST establish identity through three core pillars:
*   **Account:** The cryptographic identity (`ACC-*` form) authenticated via `ZQK_API_KEY` / `ZQK_ADMIN_API_KEY` (legacy `account:*` ids are retired — POL-AGENT-ACCOUNT-LOGIN-001).
*   **Role:** The authorization matrix defining system-level capabilities and access boundaries.
*   **Persona:** The semantic operational domain (e.g., `Design Session TPM`, `Pedantic Code Inspector`, `Coder Agent`).

## 2. Semantic Scoping via Vocabulary Schemes
To solve semantic collision (e.g., the word "record" meaning a noun vs. an action), ZQK utilizes `vocabulary_scheme` objects to act as isolated context bubbles.
*   A `persona` is tightly bound to specific `vocabulary_scheme` objects.
*   **Example Context A:** In `VOC-Implementation-Semantics`, "Validation" means executing Go compiler tests. 
*   **Example Context B:** In `VOC-Graph-Management`, "Validation" means verifying YAML schema structure.
*   By scoping definitions to schemes, we eliminate the risk of a non-technical agent hallucinating that it needs to write compilation scripts.

## 3. Just-In-Time Context Budgeting
Agents are never handed the entire knowledge graph. Using ZQK's weighted budget allocator (`pkg/agentprompt`), the Orchestrator dynamically resolves the agent's active `vocabulary_scheme` and injects *only* that specific semantic dictionary into the agent's context window payload.

## 4. Persona-Aware CLI & MCP Filtering (Invisible Scoping)
The ZQK CLI acts as an invisible data-minimization layer. When an agent queries the kernel (e.g., `zqk object list criteria` or via an MCP tool):
1. The CLI reads the agent's identity context.
2. It automatically filters the graph data based on the agent's persona and active semantic rules.
3. The agent receives *only* the pure necessities required for their domain.
*   **Outcome:** Context windows are protected from irrelevant spam, and security boundaries are structurally enforced. 

## 5. Phase-Bound Creativity
The degree of an agent's freedom is inversely proportional to the maturity of the lifecycle phase.
*   **Design Inception Phase:** Creativity is maximized. Personas (like Architects and TPMs) have broader access to semantic vocabulary schemes to explore, hypothesize, and design object structures.
*   **Implementation Phase:** Creativity is terminated. Pedantic reality takes over. Coders and Fixers are restricted to execution semantics and strict architectural mandates. The CLI filters out overarching strategic noise, forcing absolute focus on the definition of done.
