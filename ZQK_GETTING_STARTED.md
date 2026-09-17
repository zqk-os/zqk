# Welcome to ZQK (The Zen Way)

You have successfully initialized ZQK Community Edition. This is not just another project management tool—it is a **Knowledge Kernel** designed to safely orchestrate AI agents.

## The Zen Way: Objects over Markdown
Most AI setups rely on chaotic markdown files (e.g., '.iderules' or 'prompt.txt'). Markdown is brittle and untraceable. 
ZQK uses **Objects**. Every goal, requirement, task, and policy in this project is a rigid, cryptographically-hashed object stored in the local '.csnap' graph. When an AI agent connects via MCP, it reads these objects directly. It cannot hallucinate requirements, because the core engine will reject any code that doesn't map to a valid object.

## Policies & The Validation DSL
How do you stop an AI agent from going rogue? **Policies.**

A Policy is a programmable contract. When an agent attempts to mutate the system (e.g., merging code or promoting a task status), ZQK intercepts the action and evaluates it against your Policies using the **Validation DSL**.

Example of a Validation DSL rule inside a Policy object:
```yaml
validation_rules:
  - rule: "requires access:confidential"
    enforcement: "block"
    message: "Agents cannot touch this subsystem without explicit clearance."
```

If the agent doesn't have the required clearance, ZQK's core engine throws a cryptographic validation error and blocks the transaction. **No hallucinations, no devastation.**

## Your First Steps
1. **Interactive Tutorial:** Run `zcom system start-here` to complete the interactive onboarding tutorial and understand the operational philosophy.
2. **Connect your IDE:** Run `zcom mcp serve` and point your AI assistant (IDE, Claude) to it.
3. **Set the Goal:** Run `zcom object create goal --field title="Build an awesome app"`
4. **Let the Agent Work:** Just tell your AI to "Start working on the goal." It will read the graph, break the goal into requirements, and execute them under the strict governance of your policies.

*For more CLI tools, type `zcom --help`.*
