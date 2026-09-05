# Agent Tool Selection Analysis: Direct File Write vs CLI

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: Analysis  
**Purpose**: Understand why AI agents prefer direct file writes over CLI commands

## Problem Statement

Agents consistently write YAML files directly (using `write` tool) instead of using CLI commands (`zqk object create`), causing:
- Objects not using CAS (content-addressable storage)
- Hash mismatches and integrity violations
- Cache inconsistencies
- Missing audit trails
- Validation loops

## Root Cause Analysis

### 1. Tool Availability and Accessibility

**Issue**: The `write` tool is:
- **Immediately available**: No setup required
- **Simple interface**: Just provide path and content
- **Fast execution**: Single tool call
- **Low cognitive load**: Clear, direct action

**CLI commands require**:
- Multiple steps (create temp file, run command, cleanup)
- Understanding of command syntax
- Error handling for command failures
- Context switching between tools

**Impact**: Agents naturally gravitate toward the simpler, more direct tool.

### 2. Task Instruction Semantics

**User Request**: "Create a requirement object with criteria"

**Agent Mental Model**:
- "Create object" → "Write YAML file"
- Matches the physical representation (YAML files on disk)
- Natural mapping from request to action

**Required Mental Model**:
- "Create object" → "Use CLI command"
- Requires understanding of abstraction layer
- Requires knowledge of policy requirements

**Impact**: Natural language processing favors direct file manipulation.

### 3. Pattern Matching from Codebase

**Observation**: Agents see existing YAML files in `docs/process/` and infer:
- "Files are written directly"
- "This is how objects are stored"
- "I should create files the same way"

**Reality**: 
- Existing files may have been migrated
- Some files were created before CLI enforcement
- Files represent state, not creation method

**Impact**: Agents learn from observation rather than policies.

### 4. Error Recovery Patterns

**When CLI command fails**:
- Agent sees error
- Fallback to direct file write seems like "solution"
- Direct write appears to "work" (file exists)
- Validation issues discovered later (too late)

**Impact**: Short-term success with direct writes masks long-term problems.

### 5. Lack of Immediate Feedback

**Direct File Write**:
- ✅ File appears immediately
- ✅ No immediate errors
- ✅ Task appears "complete"
- ❌ Problems discovered later (validation, CAS, etc.)

**CLI Command**:
- ❌ Requires multiple steps
- ❌ May show errors immediately
- ❌ Slower feedback loop
- ✅ Prevents problems proactively

**Impact**: Agents optimize for immediate success signals.

### 6. Policy vs. Tool Enforcement

**Current State**:
- Policies exist (POL-ONBOARD-001, POL-CODE-002)
- Policies are documentation only
- No tool-level prevention
- No automatic enforcement

**What's Missing**:
- Tool-level warnings/prevention
- Automatic CLI command generation
- Validation at write-time (not later)
- Clear error messages pointing to CLI

**Impact**: Policies are "nice to have" but not enforced.

### 7. Workflow Inertia

**Pattern Established**:
- Once direct writes happen, they become pattern
- Subsequent agents follow same pattern
- Pattern reinforces itself
- Breaking pattern requires conscious effort

**Impact**: Bad patterns become self-perpetuating.

## Contributing Factors

### Tool Design

**CRITICAL DISCOVERY**: The `write` tool is **NOT a project tool** - it's a **Cursor IDE built-in tool** that the project cannot control or modify.

1. **Write Tool is a Cursor IDE Built-In** (Not Project-Controlled)
   - Part of Cursor IDE's toolset
   - Available to all agents by default
   - Cannot be modified, disabled, or controlled by the project
   - No validation at tool level (project cannot add this)
   - No policy checking (project cannot add this)
   - No warnings for object files (project cannot add this)
   - **This changes the solution approach completely**

2. **CLI Command Tool is Complex**
   - Requires file creation first (or use MCP tools)
   - Requires command syntax knowledge
   - Requires error handling
   - Multiple tool calls needed
   - MCP tools (`zqk_object_create`) are available but may not be preferred

### Documentation

1. **Policies Buried in Documentation**
   - Not surfaced at decision point
   - Not integrated into tool descriptions
   - Not part of error messages

2. **Examples Show Direct Writes**
   - Codebase shows YAML files
   - Documentation may show file structure
   - Patterns in code suggest direct writes

### Agent Workflow

1. **No Decision Framework**
   - No clear "if object file → use CLI" rule
   - No tool selection guidance
   - No workflow enforcement

2. **No Immediate Consequences**
   - Direct writes "work" initially
   - Problems discovered later
   - No immediate negative feedback

## Proposed Solutions

### Solution 1: Tool-Level Prevention

**STATUS: NOT POSSIBLE** - The `write` tool is a Cursor IDE built-in and cannot be modified by the project.

**Original Approach** (not feasible):
- Modify `write` tool to detect object files and warn/block
- Detect `docs/process/**/*.yaml` patterns
- Check if file matches object spec patterns
- Warn or block direct writes
- Suggest CLI command instead

**Why Not Feasible**:
- `write` tool is controlled by Cursor IDE, not the project
- Project cannot modify, disable, or intercept Cursor IDE tools
- Requires Cursor IDE changes (outside project scope)

### Solution 2: Enhanced Tool Descriptions

**Approach**: Add policy references to tool descriptions

**Implementation**:
- Update `write` tool description to reference policies
- Add warnings for object file patterns
- Include CLI command examples

**Pros**:
- No tool changes needed
- Educational
- Low implementation cost

**Cons**:
- Still requires agent to read and follow
- No enforcement

### Solution 3: Improve Existing MCP Tools ⭐ RECOMMENDED

**CRITICAL INSIGHT**: MCP tools like `zqk_object_create` already exist via the CLI bridge, but agents aren't using them.

**Approach**: Make existing MCP tools more attractive and easier to use than `write`

**Implementation**:
- Enhance MCP tool descriptions with clear examples
- Add object creation workflow guidance to tool descriptions
- Ensure MCP tools are prominently featured in agent onboarding
- Improve error messages to guide agents to MCP tools
- Add policy references to MCP tool descriptions

**Pros**:
- Uses existing infrastructure (no new tool development)
- Makes existing tools more discoverable
- Reduces cognitive load (better examples/guidance)
- Project-controlled (MCP tool descriptions)
- Educational (agents learn to use existing tools)

**Cons**:
- Still requires agents to choose MCP tools over `write`
- Documentation/description improvements may not be enough
- `write` tool will always be simpler (single call vs. structured parameters)

### Solution 4: Workflow Enforcement Pattern

**Approach**: Create explicit workflow patterns with checkpoints

**Implementation**:
- "Create Object" workflow pattern
- Mandatory steps (validate → CLI → verify)
- Tool sequences that enforce pattern

**Pros**:
- Clear workflow
- Enforces correct pattern
- Educational

**Cons**:
- Requires workflow tool support
- More complex than single write

### Solution 5: Immediate Validation

**Approach**: Validate files immediately after write

**Implementation**:
- Post-write validation hook
- Check if object file → validate immediately
- Fail fast with clear error + CLI suggestion

**Pros**:
- Immediate feedback
- Catches problems early
- Educational (suggests CLI)

**Cons**:
- Still allows incorrect pattern
- Validation overhead
- May slow down workflow

## Recommended Approach

**Combination of Solutions 3 + 4 + 5** (Solution 1 not feasible, Solution 3 refocused):

1. **Improve Existing MCP Tools** (Solution 3 - Refocused) ⭐ PRIMARY
   - MCP tools like `zqk_object_create` already exist via CLI bridge
   - Enhance tool descriptions with clear examples
   - Add policy references (POL-ONBOARD-001, POL-CODE-002)
   - Improve discoverability in agent onboarding
   - Make MCP tools more attractive than `write`

2. **Workflow Pattern** (Solution 4)
   - Document "Create Object" workflow
   - Emphasize: Use MCP tools (`zqk_object_create`) not `write`
   - Make it the default pattern
   - Integrate into agent guidance

3. **Immediate Validation** (Solution 5)
   - Pre-commit hooks already exist (good - blocks invalid files)
   - Post-write validation (if feasible)
   - Error messages should point to MCP tools
   - Fail fast with clear error + MCP tool suggestion

**Key Insight**: MCP tools already exist via CLI bridge. The challenge is making them the preferred choice over `write`.

**Note**: Solution 1 (tool-level prevention) is NOT feasible because `write` is a Cursor IDE built-in tool that cannot be modified by the project.

## Implementation Priority

1. **Immediate**: Enhance Existing MCP Tool Descriptions (Solution 3 - Refocused) ⭐
   - MCP tools already exist via CLI bridge
   - Improve tool descriptions with examples
   - Add policy references (POL-ONBOARD-001, POL-CODE-002)
   - Emphasize: Use MCP tools, not `write` tool
   - Make MCP tools more discoverable

2. **Immediate**: Enhanced workflow patterns (Solution 4)
   - Document "Create Object" workflow
   - Update agent onboarding to emphasize MCP tools
   - Make MCP tools the default recommendation

3. **Short-term**: Error message improvements (Solution 5)
   - Pre-commit hooks already exist (good)
   - Update error messages to reference MCP tools
   - Fail fast with MCP tool suggestions

4. **Ongoing**: Validation and monitoring (Solution 5)
   - Pre-commit hooks (already exist)
   - CI/CD checks
   - Monitor tool usage patterns

**Note**: Solution 1 (tool-level prevention) is NOT feasible - `write` is a Cursor IDE built-in tool.

## Questions for Investigation

1. **Why is the `write` tool selected over CLI commands?**
   - Tool availability?
   - Simplicity?
   - Pattern matching?
   - Error recovery?

2. **What would make CLI commands more attractive?**
   - Easier syntax?
   - Better tool integration?
   - Immediate feedback?
   - Clearer examples?

3. **How can we make policies more actionable?**
   - Tool-level integration?
   - Error message references?
   - Workflow enforcement?
   - Immediate validation?

4. **What feedback mechanisms would help?**
   - Immediate validation?
   - Pre-write warnings?
   - Post-write checks?
   - Workflow checkpoints?

## Next Steps

1. ✅ Analyze tool selection patterns (COMPLETE - this document)
2. ✅ Understand MCP bridge architecture (COMPLETE - MCP tools already exist)
3. **Investigate why agents choose `write` over `zqk_object_create`**
   - Review MCP tool descriptions
   - Check tool discoverability
   - Analyze agent onboarding documentation
4. **Enhance MCP tool descriptions**
   - Add clear examples for `zqk_object_create`
   - Add policy references (POL-ONBOARD-001, POL-CODE-002)
   - Emphasize: Use MCP tools, not `write`
5. **Update agent onboarding**
   - Emphasize MCP tools as primary method
   - Make workflow explicit: MCP tools → CLI commands → (last resort) `write`
6. **Enhance error messages**
   - Point to MCP tools in validation errors
   - Pre-commit hooks already exist (good)

## Key Insights

1. **The `write` tool is a Cursor IDE built-in tool, not a project tool.**
   - Project cannot modify, disable, or intercept it
   - Solutions must focus on making MCP tools preferred

2. **MCP tools already exist via CLI bridge.**
   - `zqk_object_create`, `zqk_object_update`, etc. are available
   - Scoped by privilege and configuration
   - The challenge is making them the preferred choice over `write`

3. **The real question**: Why do agents choose `write` over `zqk_object_create`?
   - Tool simplicity? (`write` is single call, MCP tools require structured params)
   - Discoverability? (agents may not know MCP tools exist)
   - Documentation? (onboarding may not emphasize MCP tools)
   - Cognitive load? (`write` requires less knowledge)
