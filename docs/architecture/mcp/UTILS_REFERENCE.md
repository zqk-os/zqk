# MCP Package Utilities Reference

**Last Verified:** 2026-08-31


## Overview

Parser and extractor functions have been consolidated into standardized utility files for better discoverability and maintainability. All utilities are in the `pkg/mcp` package and follow consistent naming conventions.

## Utility Files

### `utils_path.go` - Path and Command Path Utilities

**Purpose**: Extract and parse command paths, groups, and references.

**Functions**:
- `ExtractGroupFromPath(path string) string`
  - Extracts the command group from a path (e.g., "object list" -> "object")
  - Returns empty string if path has only one word

- `ExtractCommandPath(args map[string]any) (string, error)`
  - Extracts command path from tool arguments
  - Handles both `_command_path` and legacy `command`/`subcommand` format
  - Returns normalized command path

- `ParseReference(ref string) (kind, id string)`
  - Parses reference strings (e.g., "backlog_item:BLI-123" -> ("backlog_item", "BLI-123"))
  - Handles both "kind:id" and "id" formats

**Usage Example**:
```go
group := ExtractGroupFromPath("object list") // Returns "object"
commandPath, err := ExtractCommandPath(args)
kind, id := ParseReference("backlog_item:BLI-123")
```

---

### `utils_security.go` - Security Context Extractors

**Purpose**: Extract security-related information from maps (client info, account objects, etc.).

**Functions**:
- `ExtractAccountID(clientInfo map[string]any) string`
  - Extracts account ID with fallback to alternative field names
  - Returns empty string if not found (security: no default to system)

- `ExtractRoles(data map[string]any) []string`
  - Generic function to extract roles from any map
  - Handles both array and comma-separated string formats

- `ExtractPermissions(data map[string]any) []string`
  - Generic function to extract permissions from any map
  - Handles both array and comma-separated string formats

- `ExtractRolesFromClientInfo(clientInfo map[string]any) []string`
  - Convenience wrapper for client info specifically

- `ExtractPermissionsFromClientInfo(clientInfo map[string]any) []string`
  - Convenience wrapper for client info specifically

- `ExtractRolesFromAccount(account map[string]any) []string`
  - Convenience wrapper for account objects specifically

- `ExtractPermissionsFromAccount(account map[string]any) []string`
  - Convenience wrapper for account objects specifically

- `ExtractPrivilegeTags(obj map[string]any) []string`
  - Extracts privilege tags (tags starting with "access:" or "privilege:")

- `ExtractAccessPermissions(permissions []string) []string`
  - Filters permissions to only those starting with "access:"

**Usage Example**:
```go
accountID := ExtractAccountID(clientInfo)
roles := ExtractRolesFromClientInfo(clientInfo)
permissions := ExtractPermissionsFromAccount(account)
privilegeTags := ExtractPrivilegeTags(obj)
```

---

### `utils_command.go` - Command Annotation and Argument Extractors

**Purpose**: Extract information from CLI commands and tool arguments.

**Functions**:
- `ExtractPermissionsFromCommand(cmd *cobra.Command) []string`
  - Extracts permissions from command annotations (`mcp.permissions`)
  - Parses comma-separated values

- `ExtractRolesFromCommand(cmd *cobra.Command) []string`
  - Extracts roles from command annotations (`mcp.roles`)
  - Parses comma-separated values

- `ExtractPositionalArguments(args map[string]any, server *Server) []string`
  - Extracts positional arguments from tool arguments
  - Uses configurable positional argument names (defaults: "id", "ids", "kind", "kinds", "path")
  - Handles both string and array values

- `ExtractFlags(args map[string]any, server *Server) []string`
  - Extracts flag arguments from tool arguments
  - Converts to CLI flag format (--flag value)
  - Handles boolean, string, and array values
  - Excludes positional arguments and internal fields

**Usage Example**:
```go
perms := ExtractPermissionsFromCommand(cmd)
roles := ExtractRolesFromCommand(cmd)
positionalArgs := ExtractPositionalArguments(args, server)
flags := ExtractFlags(args, server)
```

---

### `utils_jsonld.go` - JSON-LD Type Mappers

**Purpose**: Map types between different formats (pflag, XSD, elicitation types).

**Functions**:
- `MapFlagTypeToJSONLDType(flagType string) string`
  - Maps pflag types to JSON-LD/XSD types
  - Returns XSD type (e.g., "xsd:boolean", "xsd:string")

- `ParseDefaultValue(defValue, flagType string) any`
  - Parses default value string to appropriate type
  - Returns typed value based on flag type

- `MapFieldTypeToXSD(fieldType string) string`
  - Maps ZQK field types to XSD types
  - Returns XSD type or custom type (e.g., "zqk:Enum", "zqk:Reference")

- `MapFieldTypeToElicitationType(fieldType string) string`
  - Maps field types to elicitation parameter types
  - Used for interactive field elicitation

**Usage Example**:
```go
jsonldType := MapFlagTypeToJSONLDType("bool") // Returns "xsd:boolean"
defaultVal := ParseDefaultValue("true", "bool") // Returns true
xsdType := MapFieldTypeToXSD("enum") // Returns "zqk:Enum"
```

---

### `utils_output.go` - Output Parsing Utilities

**Purpose**: Parse and extract JSON from command output, handling pollution.

**Functions**:
- `ExtractFirstCompleteJSONObject(output string) string`
  - Extracts the first complete JSON object from output
  - Validates that extracted JSON is not a debug log event
  - Handles multi-line JSON and whitespace

- `IsValidNonDebugJSON(jsonStr string) bool`
  - Checks if JSON string is a valid non-debug JSON object
  - Filters out debug log events

- `ParseCommandOutput(output string, stderr bytes.Buffer) map[string]any`
  - Parses command output as JSON
  - Returns parsed result or error information using CommandResultBuilder
  - Note: For full command execution context, use `parseCommandOutput` in `cli_bridge_command_execution.go`

**Usage Example**:
```go
jsonStr := ExtractFirstCompleteJSONObject(output)
if IsValidNonDebugJSON(jsonStr) {
    result := ParseCommandOutput(jsonStr, stderr)
}
```

---

## Backward Compatibility

All original function names are preserved as convenience wrappers that call the new utility functions. This ensures existing code continues to work without changes.

**Example**:
```go
// Old way (still works):
group := extractGroupFromPath(path)

// New way (preferred):
group := ExtractGroupFromPath(path)
```

## Finding Utilities

When looking for a parser or extractor function:

1. **Path/Command related** → `utils_path.go`
2. **Security/Roles/Permissions** → `utils_security.go`
3. **Command annotations/arguments** → `utils_command.go`
4. **Type mapping (JSON-LD, XSD)** → `utils_jsonld.go`
5. **Output parsing** → `utils_output.go`

All utility functions are exported (start with capital letter) and have comprehensive documentation.

## Migration Guide

When adding new extractor/parser functions:

1. Determine the appropriate utility file based on category
2. Use consistent naming: `Extract*`, `Parse*`, `Map*`
3. Add comprehensive documentation
4. Keep original function as wrapper if it's used elsewhere
5. Update this reference document
