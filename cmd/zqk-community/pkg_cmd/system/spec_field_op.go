package system

import "github.com/lanceman/zqk/pkg/aliases"

// FieldOp* are canonical wire values for spec field lifecycle operations after alias
// resolution (ResolveSpecFieldOperation). Use these for switches, metrics, and comparisons
// instead of raw string literals.
const (
	FieldOpCreate    = "create"
	FieldOpModify    = "modify"
	FieldOpDeprecate = "deprecate"
	FieldOpArchive   = "archive"
	FieldOpDelete    = "delete"
)

// specFieldOperationAliases maps user-facing names to FieldOp* (same surface CLI and SpecWriter use).
// Not pkg/cli.AliasRegistry: that table is CRUD-oriented (e.g. modify → update), which
// collides with spec field lifecycle "modify".
var specFieldOperationAliases = map[string]string{
	"define": FieldOpCreate,
	"add":    FieldOpCreate,
}

// ResolveSpecFieldOperation returns the canonical FieldOp* for a flag or API string
// (e.g. define/add → FieldOpCreate). Use at CLI boundaries and when building FieldOperation.
func ResolveSpecFieldOperation(s string) string {
	return aliases.ResolveStatic(s, specFieldOperationAliases)
}

// FieldOpRequiresDefinitionFile reports whether the operation needs a sidecar field-definition YAML
// (same rule as runFieldOperation: create/modify paths).
func FieldOpRequiresDefinitionFile(op string) bool {
	c := ResolveSpecFieldOperation(op)
	return c == FieldOpCreate || c == FieldOpModify
}

// ResolvedOperation returns the canonical FieldOp* for dispatch; Operation may be an alias.
func (op FieldOperation) ResolvedOperation() string {
	return ResolveSpecFieldOperation(op.Operation)
}
