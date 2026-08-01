package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

const (
	commandSpecExtYAML                        = ".yaml"
	commandSpecExtYML                         = ".yml"
	commandSpecFilenameTokenCommand           = "command"
	commandSpecLogMsgParseInfoFailed          = "Failed to parse command spec info"
	commandSpecLogFieldFile                   = "file"
	commandSpecFieldOperationType             = "operation_type"
	commandSpecPropertyName                   = "name"
	commandSpecPropertyType                   = "type"
	commandSpecPropertyFilePath               = "file_path"
	commandSpecPropertyTargetKind             = "target_kind"
	commandSpecPropertyShort                  = "short"
	commandSpecPropertyDescription            = "description"
	commandSpecPropertyArgsType               = "args_type"
	commandSpecPropertyArgsCount              = "args_count"
	commandSpecPropertyAliases                = "aliases"
	commandSpecPropertySynonyms               = "synonyms"
	commandSpecPropertyDataInput              = "data_input"
	commandSpecPropertyUpdateFlags            = "update_flags"
	commandSpecPropertyDryRun                 = "dry_run"
	commandSpecPropertyCascade                = "cascade"
	commandSpecPropertyUnlinkReferences       = "unlink_references"
	commandSpecPropertyQueryFlags             = "query_flags"
	commandSpecPropertyListHarnessFlags       = "list_harness_flags"
	commandSpecPropertyCountHarnessFlags      = "count_harness_flags"
	commandSpecPropertyFieldsHarnessFlags     = "fields_harness_flags"
	commandSpecPropertyFieldsIncludeListKinds = "fields_include_list_kinds"
	commandSpecEdgePropertyRelation           = "relationship"
	commandSpecLogMsgCreateEdgeFail           = "Failed to create edge"
	commandSpecLogFieldFrom                   = "from"
	commandSpecLogFieldTo                     = "to"
	commandSpecLogFieldType                   = "type"
	commandSpecQueryParamTargetKind           = "targetKind"
	commandSpecQueryParamOperation            = "operationType"
	commandSpecQueryParamName                 = "name"
	commandSpecQueryReturnAliasCmd            = "cmd"
	commandSpecTypeRegularCommand             = "command"
	commandSpecTypeCRUDCommand                = "crud_command"
	commandSpecEdgeTypeObjectKind             = "object_kind"
	commandSpecEdgeTypeAlias                  = "alias"
	commandSpecEdgeTypeSynonym                = "synonym"
	commandSpecEdgeTypeSubcommand             = "subcommand"
	commandSpecEdgeTypeOperation              = "operation"
	commandSpecEdgeTokenSeparator             = ":"
)

var commandSpecCommonKinds = []string{
	objects.KindBacklogItem,
	objects.KindRequirement,
	objects.KindGoal,
	objects.KindMilestone,
	objects.KindComponent,
	objects.KindCriteria,
}

// CommandSpecGraph represents the graph structure of command specifications
// This enables relationship discovery and GraphRAG understanding
type CommandSpecGraph struct {
	specsDir string
	specs    map[string]*CommandSpecInfo // command name -> spec info
	edges    map[string][]string         // command name -> list of related commands/objects
}

// CommandSpecInfo contains information about a command spec for graph analysis
type CommandSpecInfo struct {
	Name          string
	Type          string // "command" or "crud_command"
	FilePath      string
	Spec          *CommandSpec     // Full spec (loaded when needed)
	CRUDSpec      *CRUDCommandSpec // Full CRUD spec (if applicable)
	OperationType string           // For CRUD commands: "create", "read", "update", "delete", "list"
	TargetKind    string           // Object kind this command operates on (e.g., "backlog_item")
	Subcommands   []string         // Subcommand names
}

// NewCommandSpecGraph creates a new command spec graph
func NewCommandSpecGraph(specsDir string) *CommandSpecGraph {
	return &CommandSpecGraph{
		specsDir: specsDir,
		specs:    make(map[string]*CommandSpecInfo),
		edges:    make(map[string][]string),
	}
}

// BuildGraph scans all command spec files and builds the graph structure
func (csg *CommandSpecGraph) BuildGraph() error {
	if csg.specsDir == emptyValue {
		return errfmt.Errorf("specs directory not provided")
	}

	// Scan spec directory
	entries, err := os.ReadDir(csg.specsDir)
	if err != nil {
		return errfmt.Newf("failed to read spec directory").Wrap(err)
	}

	// First pass: collect all command specs
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), commandSpecExtYAML) && !strings.HasSuffix(entry.Name(), commandSpecExtYML)) {
			continue
		}

		// Skip non-command spec files
		if !strings.Contains(entry.Name(), commandSpecFilenameTokenCommand) {
			continue
		}

		specPath := filepath.Join(csg.specsDir, entry.Name())
		info, err := csg.parseCommandSpecInfo(specPath)
		if err != nil {
			// Log warning but skip files that can't be parsed
			eventLogger := logging.NewEventLogger(pkgctx.NewSystemContext())
			eventLogger.LogWarning(commandSpecLogMsgParseInfoFailed,
				logging.String(commandSpecLogFieldFile, specPath),
				logging.Error(err))
			continue
		}

		if info.Name != emptyValue {
			csg.specs[info.Name] = info
		}
	}

	// Second pass: build edges (relationships)
	// - Commands that operate on object kinds
	// - Commands with subcommands
	// - CRUD commands related to their operation types
	for name, info := range csg.specs {
		// Add edge: command -> object kind (OPERATES_ON)
		if info.TargetKind != emptyValue {
			if _, exists := csg.edges[name]; !exists {
				csg.edges[name] = []string{}
			}
			csg.edges[name] = append(csg.edges[name], edgeToken(commandSpecEdgeTypeObjectKind, info.TargetKind))
		}

		// Add semantic relationships from aliases and synonyms
		// This enables fluid traversal through aliases
		if info.Spec != nil {
			// Register aliases for semantic resolution
			for _, alias := range info.Spec.Aliases {
				if _, exists := csg.edges[name]; !exists {
					csg.edges[name] = []string{}
				}
				csg.edges[name] = append(csg.edges[name], edgeToken(commandSpecEdgeTypeAlias, alias))
			}
			// Register synonyms for target kind resolution
			for _, synonym := range info.Spec.Synonyms {
				if _, exists := csg.edges[name]; !exists {
					csg.edges[name] = []string{}
				}
				csg.edges[name] = append(csg.edges[name], edgeToken(commandSpecEdgeTypeSynonym, synonym))
			}
		}
		if info.CRUDSpec != nil {
			// Register aliases from CRUD spec
			for _, alias := range info.CRUDSpec.Aliases {
				if _, exists := csg.edges[name]; !exists {
					csg.edges[name] = []string{}
				}
				csg.edges[name] = append(csg.edges[name], edgeToken(commandSpecEdgeTypeAlias, alias))
			}
			// Register synonyms from CRUD spec
			for _, synonym := range info.CRUDSpec.Synonyms {
				if _, exists := csg.edges[name]; !exists {
					csg.edges[name] = []string{}
				}
				csg.edges[name] = append(csg.edges[name], edgeToken(commandSpecEdgeTypeSynonym, synonym))
			}
		}

		// Add edges: command -> subcommands (HAS_SUBCOMMAND)
		for _, subcmd := range info.Subcommands {
			if _, exists := csg.edges[name]; !exists {
				csg.edges[name] = []string{}
			}
			csg.edges[name] = append(csg.edges[name], edgeToken(commandSpecEdgeTypeSubcommand, subcmd))
		}

		// Add edge: CRUD command -> operation type (PERFORMS_OPERATION)
		if info.OperationType != emptyValue {
			if _, exists := csg.edges[name]; !exists {
				csg.edges[name] = []string{}
			}
			csg.edges[name] = append(csg.edges[name], edgeToken(commandSpecEdgeTypeOperation, info.OperationType))
		}
	}

	return nil
}

// parseCommandSpecInfo parses minimal info from a command spec file without full loading
func (csg *CommandSpecGraph) parseCommandSpecInfo(filePath string) (*CommandSpecInfo, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// First check if it's a CRUD spec
	var tempSpec map[string]any
	if err := yaml.Unmarshal(data, &tempSpec); err != nil {
		return nil, err
	}

	info := &CommandSpecInfo{
		FilePath: filePath,
	}

	// Check if it's a CRUD command spec
	if operationType, ok := tempSpec[commandSpecFieldOperationType].(string); ok {
		info.Type = commandSpecTypeCRUDCommand
		info.OperationType = operationType

		// Parse as CRUDCommandSpec
		var crudSpec CRUDCommandSpec
		if err := yaml.Unmarshal(data, &crudSpec); err == nil {
			info.CRUDSpec = &crudSpec
			info.Name = crudSpec.Name
			// Extract target kind from name or description
			info.TargetKind = csg.extractTargetKind(crudSpec.Name, crudSpec.Description)
		}
	} else {
		// Regular command spec
		info.Type = commandSpecTypeRegularCommand

		var spec CommandSpec
		if err := yaml.Unmarshal(data, &spec); err == nil {
			info.Spec = &spec
			info.Name = spec.Name
			// Extract target kind from name or description
			info.TargetKind = csg.extractTargetKind(spec.Name, spec.Description)
			// Extract subcommands
			if len(spec.Subcommands) > 0 {
				info.Subcommands = make([]string, len(spec.Subcommands))
				for i, subcmd := range spec.Subcommands {
					info.Subcommands[i] = subcmd.Name
				}
			}
		}
	}

	return info, nil
}

// extractTargetKind extracts the object kind a command operates on
// This is inferred from command name or description
func (csg *CommandSpecGraph) extractTargetKind(name, description string) string {
	// Common patterns:
	// - "get <id>" -> operates on objects (generic)
	// - "object get" -> operates on objects
	// - "backlog_item create" -> operates on backlog_item
	// - "delete <id>" -> operates on objects (generic)

	// Check if name contains object kind hints
	lowerName := strings.ToLower(name)
	if strings.Contains(lowerName, "object") {
		return "object" // Generic object operations
	}

	// Check description for object kind references
	lowerDesc := strings.ToLower(description)
	// Look for patterns like "backlog_item", "requirement", etc.
	// This is a simple heuristic - could be enhanced with NLP
	for _, kind := range commandSpecCommonKinds {
		if strings.Contains(lowerDesc, kind) {
			return kind
		}
	}

	return "" // Unknown or generic
}

// GetSpecInfo returns spec info for a command
func (csg *CommandSpecGraph) GetSpecInfo(commandName string) (*CommandSpecInfo, error) {
	info, exists := csg.specs[commandName]
	if !exists {
		return nil, errfmt.Errorf("command spec not found: %s", commandName)
	}
	return info, nil
}

// GetAllSpecs returns all command spec infos
func (csg *CommandSpecGraph) GetAllSpecs() map[string]*CommandSpecInfo {
	return csg.specs
}

// GetEdges returns edges for a command
func (csg *CommandSpecGraph) GetEdges(commandName string) []string {
	return csg.edges[commandName]
}

func edgeToken(edgeType, value string) string {
	return fmt.Sprintf("%s:%s", edgeType, value)
}

// StoreInGraph stores command specs as nodes in the graph backend
// This enables GraphRAG queries and relationship discovery
func (csg *CommandSpecGraph) StoreInGraph(ctx context.Context, conn provider.GraphConnection) error {
	if conn == nil {
		return errfmt.Errorf("graph connection not provided")
	}

	// Store each command spec as a node
	for name, info := range csg.specs {
		node := provider.Node{
			ID:     fmt.Sprintf("cmd_spec_%s", name),
			Labels: []string{"CommandSpec", "Spec"},
			Properties: map[string]any{
				commandSpecPropertyName:       name,
				commandSpecPropertyType:       info.Type,
				commandSpecPropertyFilePath:   info.FilePath,
				commandSpecFieldOperationType: info.OperationType,
				commandSpecPropertyTargetKind: info.TargetKind,
			},
		}

		// Add full spec data as properties (for GraphRAG)
		if info.Spec != nil {
			node.Properties[commandSpecPropertyShort] = info.Spec.Short
			node.Properties[commandSpecPropertyDescription] = info.Spec.Description
			if info.Spec.Args != nil {
				node.Properties[commandSpecPropertyArgsType] = info.Spec.Args.Type
				if info.Spec.Args.Count != nil {
					node.Properties[commandSpecPropertyArgsCount] = *info.Spec.Args.Count
				}
			}
			// Store aliases and synonyms for semantic traversal
			if len(info.Spec.Aliases) > 0 {
				node.Properties[commandSpecPropertyAliases] = info.Spec.Aliases
			}
			if len(info.Spec.Synonyms) > 0 {
				node.Properties[commandSpecPropertySynonyms] = info.Spec.Synonyms
			}
			node.Properties[commandSpecPropertyListHarnessFlags] = info.Spec.ListHarnessFlags
			node.Properties[commandSpecPropertyCountHarnessFlags] = info.Spec.CountHarnessFlags
			node.Properties[commandSpecPropertyFieldsHarnessFlags] = info.Spec.FieldsHarnessFlags
			node.Properties[commandSpecPropertyFieldsIncludeListKinds] = info.Spec.FieldsIncludeListKinds
			node.Properties[commandSpecPropertyQueryFlags] = info.Spec.QueryFlags
		}

		if info.CRUDSpec != nil {
			node.Properties[commandSpecPropertyShort] = info.CRUDSpec.Short
			node.Properties[commandSpecPropertyDescription] = info.CRUDSpec.Description
			node.Properties[commandSpecPropertyDataInput] = info.CRUDSpec.DataInput
			node.Properties[commandSpecPropertyUpdateFlags] = info.CRUDSpec.UpdateFlags
			node.Properties[commandSpecPropertyDryRun] = info.CRUDSpec.DryRun
			node.Properties[commandSpecPropertyCascade] = info.CRUDSpec.Cascade
			node.Properties[commandSpecPropertyUnlinkReferences] = info.CRUDSpec.UnlinkReferences
			node.Properties[commandSpecPropertyQueryFlags] = info.CRUDSpec.QueryFlags
			node.Properties[commandSpecPropertyListHarnessFlags] = info.CRUDSpec.ListHarnessFlags
			node.Properties[commandSpecPropertyCountHarnessFlags] = info.CRUDSpec.CountHarnessFlags
			node.Properties[commandSpecPropertyFieldsHarnessFlags] = info.CRUDSpec.FieldsHarnessFlags
			node.Properties[commandSpecPropertyFieldsIncludeListKinds] = info.CRUDSpec.FieldsIncludeListKinds
			// Store aliases and synonyms for semantic traversal
			if len(info.CRUDSpec.Aliases) > 0 {
				node.Properties[commandSpecPropertyAliases] = info.CRUDSpec.Aliases
			}
			if len(info.CRUDSpec.Synonyms) > 0 {
				node.Properties[commandSpecPropertySynonyms] = info.CRUDSpec.Synonyms
			}
		}

		// Create node in graph
		if err := conn.CreateNode(ctx, node); err != nil {
			return errfmt.Errorf("failed to create node for command %s: %w", name, err)
		}

		// Create edges for relationships
		for _, edgeTarget := range csg.edges[name] {
			edgeType, targetID, ok := strings.Cut(edgeTarget, commandSpecEdgeTokenSeparator)
			if !ok {
				continue
			}

			// Determine target node ID based on edge type
			var targetNodeID string
			switch edgeType {
			case commandSpecEdgeTypeObjectKind:
				// Link to ObjectSpec node
				targetNodeID = fmt.Sprintf("spec_%s", targetID)
			case commandSpecEdgeTypeSubcommand:
				// Link to subcommand CommandSpec node
				targetNodeID = fmt.Sprintf("cmd_spec_%s", targetID)
			case commandSpecEdgeTypeOperation:
				// Link to operation type (could be a node or just a property)
				// For now, we'll create a relationship to a generic operation node
				targetNodeID = fmt.Sprintf("operation_%s", targetID)
			default:
				continue
			}

			edge := provider.Edge{
				FromID: node.ID,
				ToID:   targetNodeID,
				Type:   strings.ToUpper(edgeType),
				Properties: map[string]any{
					commandSpecEdgePropertyRelation: edgeType,
				},
			}

			if err := conn.CreateEdge(ctx, edge); err != nil {
				// Log warning but continue - edge creation might fail if target doesn't exist
				eventLogger := logging.NewEventLogger(pkgctx.NewSystemContext())
				eventLogger.LogWarning(commandSpecLogMsgCreateEdgeFail,
					logging.String(commandSpecLogFieldFrom, node.ID),
					logging.String(commandSpecLogFieldTo, targetNodeID),
					logging.String(commandSpecLogFieldType, edgeType),
					logging.Error(err))
			}
		}
	}

	return nil
}

// QueryCommandSpecs queries the graph for command specs matching criteria
func QueryCommandSpecs(ctx context.Context, conn provider.GraphConnection, filter CommandSpecFilter) ([]*CommandSpecInfo, error) {
	if conn == nil {
		return nil, errfmt.Errorf("graph connection not provided")
	}

	// Build Cypher query
	whereClauses := []string{}
	params := map[string]any{}

	if filter.TargetKind != emptyValue {
		whereClauses = append(whereClauses, "cmd.target_kind = $"+commandSpecQueryParamTargetKind)
		params[commandSpecQueryParamTargetKind] = filter.TargetKind
	}
	if filter.OperationType != emptyValue {
		whereClauses = append(whereClauses, "cmd.operation_type = $"+commandSpecQueryParamOperation)
		params[commandSpecQueryParamOperation] = filter.OperationType
	}
	if filter.Name != emptyValue {
		whereClauses = append(whereClauses, "cmd.name CONTAINS $"+commandSpecQueryParamName)
		params[commandSpecQueryParamName] = filter.Name
	}

	queryStr := "MATCH (cmd:CommandSpec)"
	if len(whereClauses) > 0 {
		queryStr += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	queryStr += " RETURN " + commandSpecQueryReturnAliasCmd

	query := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    queryStr,
		Params:   params,
	}

	result, err := conn.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, errfmt.Newf("failed to execute query").Wrap(err)
	}

	// Convert results to CommandSpecInfo
	specs := []*CommandSpecInfo{}

	// Use Nodes if available (preferred - contains full node structure)
	if len(result.Nodes) > 0 {
		for _, node := range result.Nodes {
			info := &CommandSpecInfo{}
			if name, ok := node.Properties[commandSpecPropertyName].(string); ok {
				info.Name = name
			}
			if typ, ok := node.Properties[commandSpecPropertyType].(string); ok {
				info.Type = typ
			}
			if opType, ok := node.Properties[commandSpecFieldOperationType].(string); ok {
				info.OperationType = opType
			}
			if targetKind, ok := node.Properties[commandSpecPropertyTargetKind].(string); ok {
				info.TargetKind = targetKind
			}
			specs = append(specs, info)
		}
	} else {
		// Fallback to Rows (if query returns rows instead of nodes)
		for _, row := range result.Rows {
			// Extract cmd node from row
			cmdValue, ok := row[commandSpecQueryReturnAliasCmd]
			if !ok {
				continue
			}

			// Handle if cmdValue is a Node
			var node *provider.Node
			if n, ok := cmdValue.(*provider.Node); ok {
				node = n
			} else if nMap, ok := cmdValue.(map[string]any); ok {
				// Convert map to node-like structure
				info := &CommandSpecInfo{}
				if name, ok := nMap[commandSpecPropertyName].(string); ok {
					info.Name = name
				}
				if typ, ok := nMap[commandSpecPropertyType].(string); ok {
					info.Type = typ
				}
				if opType, ok := nMap[commandSpecFieldOperationType].(string); ok {
					info.OperationType = opType
				}
				if targetKind, ok := nMap[commandSpecPropertyTargetKind].(string); ok {
					info.TargetKind = targetKind
				}
				specs = append(specs, info)
				continue
			} else {
				continue
			}

			if node != nil {
				info := &CommandSpecInfo{}
				if name, ok := node.Properties[commandSpecPropertyName].(string); ok {
					info.Name = name
				}
				if typ, ok := node.Properties[commandSpecPropertyType].(string); ok {
					info.Type = typ
				}
				if opType, ok := node.Properties[commandSpecFieldOperationType].(string); ok {
					info.OperationType = opType
				}
				if targetKind, ok := node.Properties[commandSpecPropertyTargetKind].(string); ok {
					info.TargetKind = targetKind
				}
				specs = append(specs, info)
			}
		}
	}

	return specs, nil
}

// CommandSpecFilter filters command specs for queries
type CommandSpecFilter struct {
	TargetKind    string
	OperationType string
	Name          string
}
