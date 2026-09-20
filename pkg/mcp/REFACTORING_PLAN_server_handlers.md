# Refactoring Plan: server_handlers.go

**File:** `pkg/mcp/server_handlers.go`  
**Original Size:** 2,117 lines  
**Final Size:** 182 lines (91% reduction)  
**Target:** Split into focused files (~400-600 lines each)  
**Status:** ✅ ALL PHASES COMPLETE (Phases 1-7)

## Analysis

### Function Distribution
- **Setup:** setupHandlers (routing configuration)
- **Core Handlers:** handleInitialize, handleShutdown
- **List/Get Handlers:** handleToolsList, handleResourcesList, handleResourcesGet, handlePromptsList, handlePromptsGet, handleRootsList
- **Tool Execution:** handleToolsCall
- **Event Handlers:** handleEventsSubscribe, handleEventsUnsubscribe, handleEventsList
- **Notification Handlers:** handleNotificationInitialized, handleNotificationCancelled
- **Authentication:** createAuthenticationSession, updateAuthenticationSession, validateCredentialsAndResolveAccount, validateUsernamePassword, validateKeystoreKey, validateOAuthToken, validatePersonalAccessToken
- **Account Management:** loadAccountObject, loadAccountByEmail, extractRolesFromAccount, extractPermissionsFromAccount
- **Keystore Operations:** loadKeystoreEntry, updateKeystoreEntry
- **Helpers:** hasCredentials

### Proposed File Structure

#### 1. `server_handlers_core.go` (~400 lines)
**Purpose:** Core handler setup and initialization/shutdown handlers

**Contents:**
- `setupHandlers()` - Method router setup
- `handleInitialize()` - Initialize handler
- `handleShutdown()` - Shutdown handler
- Constants (client info, object fields, session fields, auth strategies, capabilities)

**Dependencies:** Core server types

---

#### 2. `server_handlers_list.go` (~300 lines)
**Purpose:** List and get operations

**Contents:**
- `handleToolsList()` - List available tools
- `handleResourcesList()` - List available resources
- `handleResourcesGet()` - Get specific resource
- `handlePromptsList()` - List available prompts
- `handlePromptsGet()` - Get specific prompt
- `handleRootsList()` - List roots

**Dependencies:** Core

---

#### 3. `server_handlers_tools.go` (~200 lines)
**Purpose:** Tool execution handler

**Contents:**
- `handleToolsCall()` - Execute tool calls

**Dependencies:** Core, tool execution logic

---

#### 4. `server_handlers_auth.go` (~600 lines)
**Purpose:** Authentication and session management

**Contents:**
- `createAuthenticationSession()` - Create new auth session
- `updateAuthenticationSession()` - Update existing session
- `validateCredentialsAndResolveAccount()` - Validate credentials
- `validateUsernamePassword()` - Username/password validation
- `validateKeystoreKey()` - Keystore key validation
- `validateOAuthToken()` - OAuth token validation
- `validatePersonalAccessToken()` - PAT validation
- `hasCredentials()` - Check if credentials present
- `loadAccountObject()` - Load account by ID
- `loadAccountByEmail()` - Load account by email
- `extractRolesFromAccount()` - Extract roles
- `extractPermissionsFromAccount()` - Extract permissions

**Dependencies:** Core, storage, validation

---

#### 5. `server_handlers_keystore.go` (~200 lines)
**Purpose:** Keystore operations

**Contents:**
- `loadKeystoreEntry()` - Load keystore entry
- `updateKeystoreEntry()` - Update keystore entry

**Dependencies:** Core, storage

---

#### 6. `server_handlers_events.go` (~200 lines)
**Purpose:** Event subscription handlers

**Contents:**
- `handleEventsSubscribe()` - Subscribe to events
- `handleEventsUnsubscribe()` - Unsubscribe from events
- `handleEventsList()` - List subscriptions
- `NotificationSentinel` type and Error() method

**Dependencies:** Core, event emitter

---

#### 7. `server_handlers_notifications.go` (~200 lines)
**Purpose:** Notification handlers

**Contents:**
- `handleNotificationInitialized()` - Handle initialized notification
- `handleNotificationCancelled()` - Handle cancelled notification

**Dependencies:** Core

---

## Migration Strategy

### Phase 1: Extract List Handlers (Low Risk) - ✅ COMPLETE
1. ✅ Create `server_handlers_list.go` (901 lines)
2. ✅ Move list/get handlers (handleToolsList, handleResourcesList, handleResourcesGet, handlePromptsList, handlePromptsGet, handleRootsList)
3. ✅ Move type definitions (ResourceGetParams, PromptGetParams, PromptGetResult, PromptMessage)
4. ✅ Test - Build successful, tests passing

### Phase 2: Extract Event Handlers (Low Risk) - ✅ COMPLETE
1. ✅ Create `server_handlers_events.go` (176 lines)
2. ✅ Move event subscription handlers (handleEventsSubscribe, handleEventsUnsubscribe, handleEventsList)
3. ✅ Move NotificationSentinel type and Error() method
4. ✅ Move type definitions (EventsSubscribeParams, EventsSubscribeResult)
5. ✅ Test - Build successful, tests passing

### Phase 3: Extract Notification Handlers (Low Risk) - ✅ COMPLETE
1. ✅ Create `server_handlers_notifications.go` (216 lines)
2. ✅ Move notification handlers (handleNotificationInitialized, handleNotificationCancelled)
3. ✅ Test - Build successful, tests passing

### Phase 4: Extract Tool Handler (Low Risk) - ✅ COMPLETE
1. ✅ Create `server_handlers_tools.go` (153 lines)
2. ✅ Move handleToolsCall
3. ✅ Test - Build successful, tests passing

### Phase 5: Extract Authentication (Medium Risk) - ✅ COMPLETE
1. ✅ Create `server_handlers_auth.go` (541 lines)
2. ✅ Move authentication and account management functions:
   - createAuthenticationSession, updateAuthenticationSession
   - validateCredentialsAndResolveAccount, validateUsernamePassword
   - validateKeystoreKey, validateOAuthToken, validatePersonalAccessToken
   - loadAccountObject, loadAccountByEmail
   - extractRolesFromAccount, extractPermissionsFromAccount
   - loadEnabledAuthStrategies, isStrategyStatusActive
   - hasCredentials
   - loadKeystoreEntry, updateKeystoreEntry (keystore functions used by auth)
3. ✅ Test - Build successful, tests passing

### Phase 6: Extract Keystore (Low Risk) - ✅ COMPLETE (merged into Phase 5)
**Note:** Keystore functions (loadKeystoreEntry, updateKeystoreEntry) were moved to server_handlers_auth.go in Phase 5 since they're used by authentication validation.

### Phase 7: Finalize Core (Low Risk) - ✅ COMPLETE
1. ✅ Verified core file contains only:
   - Shared constants (client info, object fields, session fields, auth strategies, capabilities)
   - setupHandlers() - Method router setup
   - handleInitialize() - Initialization handler
   - handleShutdown() - Shutdown handler
2. ✅ Final testing - All builds and tests passing
3. ✅ File reduced from 2,117 to 182 lines (91% reduction)

## Testing Strategy

1. **Unit Tests:** Each extracted file should have corresponding test file
2. **Integration Tests:** Full handler operations
3. **Regression Tests:** Run full test suite after each phase
4. **Authentication Tests:** Thoroughly test all auth paths

## Risk Mitigation

- **Incremental:** One file at a time
- **Test After Each Phase:** Don't proceed until tests pass
- **Review Dependencies:** Ensure imports are correct
- **Authentication Critical:** Phase 5 requires extra care due to security implications
