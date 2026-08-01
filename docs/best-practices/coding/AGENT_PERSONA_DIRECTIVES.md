# Agent Persona Directives

**Status**: Active  
**Last Updated**: 2026-06-12

This document establishes the strict standards for defining and implementing Sub-Agent Persona Directives within the ZQK ecosystem.

## Core Principles

When defining directives for sub-agents (e.g., Code Reviewer, Debugger, Researcher), you MUST adhere to the following strict requirements:

### 1. Declarative Form
Persona directives must be defined declaratively, typically in YAML or strict Markdown formats, rather than as loose prompt fragments or imperative scripts. This ensures they can be parsed, validated, and loaded deterministically.

### 2. Deterministic Behavior
Agents must exhibit deterministic, predictable behavior. Directives should define clear constraints, expected inputs, and exact output formats. Avoid open-ended or ambiguous instructions that can lead to drift.

### 3. Proper Scoping via RBAC
Every sub-agent persona must operate within a bounded context. Directives must explicitly map to an RBAC role or security context. Sub-agents should only have the minimum permissions necessary to accomplish their task (e.g., read-only for codebase researchers).

### 4. Securely Versioned
Directives are code. They must be securely versioned in the repository alongside application code. Any changes to a persona directive must go through the standard pull request review process to prevent unintended behavioral changes or privilege escalation.

## Example: Proper Persona Directive

Here is an example of a properly structured persona directive in YAML:

```yaml
kind: AgentPersona
metadata:
  name: codebase-researcher
  version: v1.2.0
spec:
  description: "Responsible for safely analyzing the codebase without modifying files."
  rbac:
    role: "readonly-researcher"
    allowed_tools:
      - grep_search
      - view_file
      - list_dir
  directive: |
    You are the Codebase Researcher. Your primary responsibility is to analyze the codebase and provide detailed, accurate information to the caller.
    
    Constraints:
    1. You are strictly read-only. Do not attempt to modify, create, or delete any files.
    2. Respond with precise file paths and line numbers.
    3. Always format your findings using standard GitHub-flavored Markdown.
    
    Expected Output:
    Return a structured report summarizing the findings, followed by relevant code snippets.
```

## Violations
- **DO NOT** hardcode sub-agent prompt strings directly in Go code or scripts.
- **DO NOT** grant wild-card (`*`) permissions or write access to a read-only persona.
- **DO NOT** provide open-ended instructions like "Help out the user however you can."
