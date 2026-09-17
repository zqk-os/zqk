package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// toLabel converts a snake_case kind to PascalCase label
// e.g., "backlog_item" -> "BacklogItem"
// Returns empty string if the kind contains invalid characters to prevent Cypher injection.
func toLabel(kind string) string {
	parts := strings.Split(kind, "_")
	var labelParts []string
	for _, part := range parts {
		if part != emptyValue {
			labelParts = append(labelParts, strings.ToUpper(part[:1])+strings.ToLower(part[1:]))
		}
	}
	label := strings.Join(labelParts, "")
	for _, r := range label {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') {
			return "" // Reject unsafe label
		}
	}
	return label
}

// buildCypherCondition builds a Cypher condition for a filter operator
func (g *GraphObjectStorage) buildCypherCondition(field string, operator FilterOperator, paramName string) string {
	fieldExpr := fmt.Sprintf("n.%s", field)
	// Memgraph strictly types newer objects as zoned_date_time, while older ones are strings.
	// To safely compare inequalities without type mismatches, cast all date fields to string.
	// ISO 8601 strings compare correctly lexicographically.
	if strings.HasSuffix(field, "_at") {
		fieldExpr = fmt.Sprintf("toString(n.%s)", field)
	}

	switch operator {
	case OpEqual:
		return fmt.Sprintf("%s = $%s", fieldExpr, paramName)
	case OpNotEqual:
		return fmt.Sprintf("%s <> $%s", fieldExpr, paramName)
	case OpGreaterThan:
		return fmt.Sprintf("%s > $%s", fieldExpr, paramName)
	case OpGreaterThanOrEqual:
		return fmt.Sprintf("%s >= $%s", fieldExpr, paramName)
	case OpLessThan:
		return fmt.Sprintf("%s < $%s", fieldExpr, paramName)
	case OpLessThanOrEqual:
		return fmt.Sprintf("%s <= $%s", fieldExpr, paramName)
	case OpContains:
		return fmt.Sprintf("%s CONTAINS $%s", fieldExpr, paramName)
	case OpStartsWith:
		return fmt.Sprintf("%s STARTS WITH $%s", fieldExpr, paramName)
	case OpEndsWith:
		return fmt.Sprintf("%s ENDS WITH $%s", fieldExpr, paramName)
	case OpIn:
		return fmt.Sprintf("%s IN $%s", fieldExpr, paramName)
	case OpNotIn:
		return fmt.Sprintf("NOT %s IN $%s", fieldExpr, paramName)
	case OpHas:
		// For array contains: any(x IN n.field WHERE x = $param)
		return fmt.Sprintf("any(x IN %s WHERE x = $%s)", fieldExpr, paramName)
	case OpExists:
		// $exists: true means field exists, false means it doesn't
		return fmt.Sprintf("($%s = true AND %s IS NOT NULL) OR ($%s = false AND %s IS NULL)", paramName, fieldExpr, paramName, fieldExpr)
	case OpHasAll:
		return fmt.Sprintf("all(x IN $%s WHERE x IN %s)", paramName, fieldExpr)
	case OpHasAny:
		return fmt.Sprintf("any(x IN $%s WHERE x IN %s)", paramName, fieldExpr)
	case OpIsNull:
		// $isNull: true means field is null, false means it's not null
		return fmt.Sprintf("($%s = true AND (%s IS NULL OR NOT EXISTS(%s))) OR ($%s = false AND %s IS NOT NULL)", paramName, fieldExpr, fieldExpr, paramName, fieldExpr)
	case OpBefore:
		return fmt.Sprintf("%s < $%s", fieldExpr, paramName)
	case OpAfter:
		return fmt.Sprintf("%s > $%s", fieldExpr, paramName)
	case OpOn:
		return fmt.Sprintf("%s = $%s", fieldExpr, paramName)
	case OpOnOrBefore:
		return fmt.Sprintf("%s <= $%s", fieldExpr, paramName)
	case OpOnOrAfter:
		return fmt.Sprintf("%s >= $%s", fieldExpr, paramName)
	case OpBetween, OpWithin:
		return fmt.Sprintf("(%s >= $%s[0] AND %s <= $%s[1])", fieldExpr, paramName, fieldExpr, paramName)
	case OpRegex:
		return fmt.Sprintf("%s =~ $%s", fieldExpr, paramName)
	default:
		// Unknown operator, default to equality
		return fmt.Sprintf("%s = $%s", fieldExpr, paramName)
	}
}

// objectToNode converts an object map to a graph node
func (g *GraphObjectStorage) objectToNode(obj map[string]any) (provider.Node, error) {
	id, ok := obj[objects.FieldKeyID].(string)
	if !ok || id == emptyValue {
		return provider.Node{}, errfmt.Errorf(ConstStreamObjectMustHaveAnIdField)
	}

	kind, ok := obj[objects.FieldKeyKind].(string)
	if !ok || kind == emptyValue {
		return provider.Node{}, errfmt.Errorf(ErrMsgObjectNeedsKind)
	}

	// Convert kind to label
	label := toLabel(kind)

	// Create node with Entity label and kind-specific label
	node := provider.Node{
		ID:         id,
		Labels:     []string{label, "Entity"},
		Properties: make(map[string]any),
	}

	// Apply Bucketing Strategy for Graph Nodes
	if g.bucketRegistry != nil {
		bucketKey := g.bucketRegistry.GetBucketKey(kind, obj, "")
		if bucketKey != "" {
			// Convert bucket key to PascalCase label (e.g., "2026-07" -> "202607")
			safeBucketKey := strings.ReplaceAll(bucketKey, "-", "")
			safeBucketKey = strings.ReplaceAll(safeBucketKey, "_", "")
			node.Labels = append(node.Labels, label+"_"+safeBucketKey)
		}
	}

	// Copy all properties from object to node, converting temporal values to RFC3339 string
	for k, v := range obj {
		// Skip id (already set) and kind (represented as label)
		if k == objects.FieldKeyID || k == objects.FieldKeyKind {
			continue
		}
		var val any = v
		if t, ok := v.(time.Time); ok {
			val = zqktime.FormatRFC3339UTC(t)
		} else if tp, ok := v.(*time.Time); ok {
			val = zqktime.FormatRFC3339UTCPtr(tp)
		}
		node.Properties[k] = val
	}

	// Apply semantic compression
	if g.projectRoot != "" {
		policy := GetCompressionPolicy(g.projectRoot, kind)
		if policy != nil && policy.CompressFieldKeys {
			registry := GetFieldRegistry(g.projectRoot)
			if registry != nil {
				node.Properties = registry.TranslateFieldKeys(node.Properties)
			}
		}
	}

	return node, nil
}

// nodeToObject converts a graph node to an object map
func (g *GraphObjectStorage) nodeToObject(node *provider.Node) map[string]any {
	if node == nil {
		return nil
	}

	obj := make(map[string]any)

	// Set id
	obj[objects.FieldKeyID] = node.ID

	// Infer kind from labels (find the non-Entity label)
	var kind string
	for _, label := range node.Labels {
		if label != "Entity" {
			// Convert PascalCase label back to snake_case kind
			kind = labelToKind(label)
			break
		}
	}
	if kind == emptyValue {
		// Fallback: use first label
		if len(node.Labels) > 0 {
			kind = labelToKind(node.Labels[0])
		}
	}
	obj[objects.FieldKeyKind] = kind

	// Copy all properties, converting temporal values (e.g. from zoned_date_time) to RFC3339 string
	for k, v := range node.Properties {
		var val any = v
		if t, ok := v.(time.Time); ok {
			val = zqktime.FormatRFC3339UTC(t)
		} else if tp, ok := v.(*time.Time); ok {
			val = zqktime.FormatRFC3339UTCPtr(tp)
		}
		obj[k] = val
	}

	// Apply semantic decompression
	if g.projectRoot != "" && kind != "" {
		policy := GetCompressionPolicy(g.projectRoot, kind)
		if policy != nil && policy.CompressFieldKeys {
			registry := GetFieldRegistry(g.projectRoot)
			if registry != nil {
				obj = registry.RestoreFieldKeys(obj)
			}
		}
	}

	// Graph drivers often return lists as []any; normalize *_refs to []string for file parity.
	for k := range obj {
		if strings.HasSuffix(k, "_refs") {
			if _, ok := obj[k].([]any); ok {
				_ = NormalizeGraphListField(obj, k)
			}
		}
	}

	return obj
}

// NormalizeGraphListField coerces graph-round-tripped list fields ([]any) to []string
// so callers comparing file vs graph reads share one shape for *_refs slices.
func NormalizeGraphListField(obj map[string]any, key string) []string {
	if obj == nil {
		return nil
	}
	switch refs := obj[key].(type) {
	case []string:
		return refs
	case []any:
		out := make([]string, 0, len(refs))
		for _, ref := range refs {
			if str, ok := ref.(string); ok && str != "" {
				out = append(out, str)
			}
		}
		obj[key] = out
		return out
	}
	return nil
}

// labelToKind converts a PascalCase label to snake_case kind
// e.g., "BacklogItem" -> "backlog_item"
func labelToKind(label string) string {
	var parts []string
	var current strings.Builder

	for i, r := range label {
		if i > 0 && r >= 'A' && r <= 'Z' {
			// New word starts
			if current.Len() > 0 {
				parts = append(parts, strings.ToLower(current.String()))
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		parts = append(parts, strings.ToLower(current.String()))
	}

	return strings.Join(parts, "_")
}

// checkPermission checks if the security context has permission for the operation
// Uses shared CheckPermission utility for consistency
func (g *GraphObjectStorage) checkPermission(secCtx *pkgctx.SecurityContext, operation, kind string) error {
	return CheckPermission(secCtx, operation, kind)
}

// ensureObjectMetadata ensures required metadata fields are set
// Uses shared EnsureObjectMetadata utility for consistency
func (g *GraphObjectStorage) ensureObjectMetadata(ctx context.Context, obj map[string]any, secCtx *pkgctx.SecurityContext, isCreate bool) {
	adapter := &IDValidatorAdapter{IDValidator: g.idValidator}
	EnsureObjectMetadata(ctx, obj, secCtx, isCreate, adapter)
}

// ensureObjectID ensures object has a valid ID, generating one if needed.
// For graph storage, we currently use timestamp-based IDs with a random component
// for all kinds to ensure global uniqueness and avoid counter contention.
func (g *GraphObjectStorage) ensureObjectID(_ context.Context, obj map[string]any, kind string) (string, error) {
	id, ok := obj[objects.FieldKeyID].(string)
	if !ok || id == emptyValue {
		// Get ID prefix from validator
		if err := g.idValidator.LoadPatterns(); err != nil {
			return "", errfmt.Newf(ConstStreamFailedToLoadIdPatterns).Wrap(err)
		}
		prefixes := g.idValidator.GetValidPrefixes(kind)
		if len(prefixes) == 0 {
			return "", errfmt.Errorf(ErrMsgNoValidIDPrefix, kind)
		}
		prefix := NormalizeCASIDPrefix(prefixes[0])

		// Generate unique timestamp-based ID with random component
		// Format: PREFIX-timestamp-random (e.g., BAS-1768909936457275000-a1b2c3d4)
		baseTime := time.Now().UnixNano()
		randomBytes := make([]byte, 4)
		if _, err := rand.Read(randomBytes); err != nil {
			// Fallback: use timestamp only if random fails
			id = fmt.Sprintf("%s%d", prefix, baseTime)
		} else {
			randomHex := hex.EncodeToString(randomBytes)
			id = fmt.Sprintf("%s%d-%s", prefix, baseTime, randomHex)
		}
		obj[objects.FieldKeyID] = id
	}

	// Validate ID format (strict)
	if err := g.idValidator.LoadPatterns(); err != nil {
		return "", errfmt.Newf(ErrMsgLoadIDPatternsValidation).Wrap(err)
	}
	valid, err := g.idValidator.ValidateID(id, kind)
	if err != nil {
		return "", errfmt.Newf(ErrMsgValidateID).Wrap(err)
	}
	if !valid {
		return "", errfmt.Errorf(ErrMsgInvalidIDFormat, kind, id)
	}

	return id, nil
}
