# Refactoring Plan: server.go

**File:** `pkg/mcp/server.go`  
**Original Size:** 2,157 lines  
**Current Size:** 405 lines (81% reduction from original)  
**Target:** Split into focused files (~500-800 lines each)  
**Status:** ✅ Complete — server.go split into server_types, server_config, server_errors, server_registration, server_tool_execution

## Analysis

### Function Distribution
- **Type Definitions:** Server, ServerConfig, ClientConnection, Tool, Resource, Prompt, Request, Response, etc.
- **Core Server Methods:** NewServer, SetConfig, SetRootCommand, SetSecurityContext, etc.
- **Registration Methods:** RegisterTool, RegisterResource, RegisterPrompt
- **List Methods:** ListTools, ListResources, ListPrompts
- **Tool Execution:** HandleToolCall, handleToolCallWithContext
- **Message Handling:** SendMessageToClient, SendMessageToClientByID
- **Shutdown Logic:** shutdownSequence, isShuttingDown, GetShutdownContext, RegisterShutdownHook
- **Trace Logging:** openClientSpecificTraceWriter, switchToClientSpecificTrace, getTraceWriter
- **Helper Functions:** mapValues, mapValuesDeref, getClientIDWithRole, CheckFormatPermission
- **Queue/Config Helpers:** canSendNotifications, getQueueConfig
- **Sequence ID Management:** generateSequenceID, getCurrentSequenceID

### Proposed File Structure

#### 1. `server_core.go` (~800 lines)
**Purpose:** Core server types, initialization, and basic operations

**Contents:**
- Type definitions (Server, ServerConfig, ClientConnection, Tool, Resource, Prompt, Request, Response, etc.)
- `NewServer()` constructor
- Basic setters/getters (SetConfig, SetRootCommand, SetSecurityContext, SetStorageProvider, SetProjectRoot, GetProjectRoot, etc.)
- Registration methods (RegisterTool, RegisterResource, RegisterPrompt)
- List methods (ListTools, ListResources, ListPrompts, getToolCount, getPromptCount)
- `HandleToolCall()` wrapper
- `Serve()` method (orchestrates lifecycle builder)
- Helper functions (mapValues, mapValuesDeref, getClientIDWithRole, CheckFormatPermission)

**Dependencies:** Minimal - core types only

---

#### 2. `server_lifecycle.go` (~600 lines)
**Purpose:** Shutdown and lifecycle management

**Contents:**
- `shutdownSequence()` - Main shutdown logic
- `isShuttingDown()` - Shutdown state check
- `GetShutdownContext()` - Get shutdown context
- `RegisterShutdownHook()` - Register shutdown hooks
- Shutdown-related helper functions
- Lifecycle state management

**Dependencies:** Core

---

#### 3. `server_trace.go` (~400 lines)
**Purpose:** Trace logging functionality

**Contents:**
- `openClientSpecificTraceWriter()` - Open client-specific trace file
- `switchToClientSpecificTrace()` - Switch to client trace
- `getTraceWriter()` - Get current trace writer
- Trace configuration helpers
- Rolling trace writer management

**Dependencies:** Core

---

#### 4. `server_messaging.go` (~300 lines)
**Purpose:** Client messaging and communication

**Contents:**
- `SendMessageToClient()` - Send message to current client
- `SendMessageToClientByID()` - Send message to specific client
- `canSendNotifications()` - Check notification capability
- `getQueueConfig()` - Get message queue configuration
- Client connection management helpers

**Dependencies:** Core

---

#### 5. `server_helpers.go` (~200 lines)
**Purpose:** Utility and helper functions

**Contents:**
- `generateSequenceID()` - Generate sequence ID
- `getCurrentSequenceID()` - Get current sequence ID
- Other utility functions
- Internal helper methods

**Dependencies:** Core

---

## Migration Strategy

### Phase 1: Extract Helpers (Low Risk)
1. Create `server_helpers.go`
2. Move utility functions (generateSequenceID, getCurrentSequenceID, etc.)
3. Test

### Phase 2: Extract Trace Logging (Low Risk)
1. Create `server_trace.go`
2. Move trace-related functions
3. Test

### Phase 3: Extract Messaging (Low Risk)
1. Create `server_messaging.go`
2. Move message sending functions
3. Test

### Phase 4: Extract Lifecycle (Medium Risk)
1. Create `server_lifecycle.go`
2. Move shutdown and lifecycle functions
3. Test thoroughly

### Phase 5: Finalize Core (Low Risk) - ✅ COMPLETE
1. ✅ Keep only core types and operations in main file
2. ✅ Verify all extracted files are properly organized
3. ✅ Final testing - All tests passing

**Completed Phases:**
- ✅ Phase 1: Extract Helpers → `server_helpers.go` (50 lines)
- ✅ Phase 2: Extract Trace Logging → `server_trace.go` (318 lines)
- ✅ Phase 3: Extract Messaging → `server_messaging.go` (298 lines)
- ✅ Phase 4: Extract Lifecycle → `server_lifecycle.go` (187 lines)

**Remaining in server.go (405 lines):**
- Server struct and NewServer
- Setters/getters (SetConfig, SetRootCommand, SetSecurityContext, etc.)
- CheckFormatPermission, getClientIDWithRole
- HandleToolCall (wrapper), Serve, GetMCPMetrics, GetMCPMetricsSnapshot
- getClientEventContext, recordClientEvent, negotiateProtocolVersion

**Extracted files:**
- `server_types.go` — ClientConnection, Tool, Resource, Prompt, Request, Response, InitializeParams/Result, ToolCallParams/Result, Content, CancelledParams, ToolHandler
- `server_config.go` — ServerConfig, AgentConfig, LoadMCPConfig, expandConfigSubstitutions, expandPathSubstitutions, expandEnvVars
- `server_errors.go` — isCriticalError, isCriticalErrorCode, getCriticalErrorCodes, SendCriticalError
- `server_registration.go` — RegisterTool, RegisterPrompt, RegisterResource, RegisterResourceWithMetadata, ListTools, ListPrompts, getToolCount, getPromptCount
- `server_tool_execution.go` — handleToolCallWithContext, executeCLICommandWithContext, BootstrapCLITools

## Testing Strategy

1. **Unit Tests:** Each extracted file should have corresponding test file
2. **Integration Tests:** Full server operations
3. **Regression Tests:** Run full test suite after each phase
4. **Performance Tests:** Ensure no performance regression

## Risk Mitigation

- **Incremental:** One file at a time
- **Test After Each Phase:** Don't proceed until tests pass
- **Keep Original:** Don't delete original until all phases complete
- **Review Dependencies:** Ensure imports are correct
