package instance_builders

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// BaseInstanceBuilder provides common functionality for instance builders.
// It is safe for concurrent use: all access to fields is protected by mu.
type BaseInstanceBuilder struct {
	kind          string
	schemaVersion string
	fieldOrder    []string // Canonical field order for sequence format
	specBuilder   SpecBuilderInterface
	fields        map[string]any // Field values being built
	mu            sync.RWMutex   // Protects fields for concurrent SetField/Build
}

const (
	templateLoremIpsum      = "Lorem ipsum dolor sit amet"
	templatePlaceholderID   = "ID-LOREM-001"
	templatePlaceholderTime = "2026-01-01T00:00:00Z"

	metricDefaultNamespaceID = "zqk:kernel"
	metricDefaultOriginValue = "zqk"
	metricDefaultStatus      = "implemented"

	baseBuilderDirectoryPerm fileutil.FileMode = 0o755
	baseBuilderFilePerm      fileutil.FileMode = 0o600

	fieldTypeEnum       = "enum"
	fieldTypeInteger    = "integer"
	fieldTypeInt        = "int"
	fieldTypeNumber     = "number"
	fieldTypeFloat      = "float"
	fieldTypeBoolean    = "boolean"
	fieldTypeBool       = "bool"
	fieldTypeArray      = "array"
	fieldTypeList       = "list"
	fieldTypeObject     = "object"
	fieldTypeMap        = "map"
	fieldTypeTimestamp  = "timestamp"
	fieldTypeDateTime   = "datetime"
	fieldTypeID         = "id"
	fieldTypeIdentifier = "identifier"

	baseBuilderSpecFileExtYAML = ".yaml"
	baseBuilderFieldValidation = "validation"
	baseBuilderFieldRequired   = "required"
	baseBuilderFieldEnum       = "enum"
)

var chronoFieldNameIndicators = []string{"time", "_at", "date"}

// NewBaseInstanceBuilder creates a new base instance builder
func NewBaseInstanceBuilder(kind, schemaVersion string, fieldOrder []string, specBuilder SpecBuilderInterface) *BaseInstanceBuilder {
	return &BaseInstanceBuilder{
		kind:          kind,
		schemaVersion: schemaVersion,
		fieldOrder:    fieldOrder,
		specBuilder:   specBuilder,
		fields:        make(map[string]any),
	}
}

// SetField sets a field value (thread-safe).
func (b *BaseInstanceBuilder) SetField(fieldName string, value any) InstanceBuilder {
	_ = concurrency.RunInLock(&b.mu, func() error {
		b.fields[fieldName] = value
		return nil
	})
	return b
}

// ID sets the ID field
func (b *BaseInstanceBuilder) ID(id string) InstanceBuilder {
	b.SetField(objects.FieldKeyID, id)
	return b
}

// SetID is a compatibility shim for legacy callsites.
func (b *BaseInstanceBuilder) SetID(id string) InstanceBuilder {
	return b.ID(id)
}

// Status sets the status field
func (b *BaseInstanceBuilder) Status(status string) InstanceBuilder {
	b.SetField(objects.FieldKeyStatus, status)
	return b
}

// SetStatus is a compatibility shim for legacy callsites.
func (b *BaseInstanceBuilder) SetStatus(status string) InstanceBuilder {
	return b.Status(status)
}

// Title sets the title field
func (b *BaseInstanceBuilder) Title(title string) InstanceBuilder {
	b.SetField(objects.FieldKeyTitle, title)
	return b
}

// SetTitle is a compatibility shim for legacy callsites.
func (b *BaseInstanceBuilder) SetTitle(title string) InstanceBuilder {
	return b.Title(title)
}

// GetKind returns the object kind this builder handles
func (b *BaseInstanceBuilder) GetKind() string {
	return b.kind
}

// GetSchemaVersion returns the schema version this builder supports
func (b *BaseInstanceBuilder) GetSchemaVersion() string {
	return b.schemaVersion
}

// ToEventMap returns the current fields as a map without validation or system field additions
// This is useful for inspecting the builder state or passing to systems that need event maps
// (e.g., buffer aggregation checks). For a complete instance, use Build() instead.
func (b *BaseInstanceBuilder) ToEventMap() map[string]any {
	var eventMap map[string]any
	_ = concurrency.RunInRLock(&b.mu, func() error {
		eventMap = make(map[string]any)
		maps.Copy(eventMap, b.fields)
		return nil
	})
	return eventMap
}

// BuildRequiredTemplate returns a minimal template map for this kind with
// required fields prefilled using type-aware lorem-style placeholder values.
func (b *BaseInstanceBuilder) BuildRequiredTemplate() (map[string]any, error) {
	spec, err := objects.GetGlobalSpecLoader().LoadSpecWithInheritance(baseBuilderSpecFileNameForKind(b.kind))
	if err != nil {
		return nil, errfmt.Errorf("load spec for %s: %w", b.kind, err)
	}

	out := map[string]any{
		objects.FieldKeyID:            templatePlaceholderID,
		objects.FieldKeyKind:          b.kind,
		objects.FieldKeySchemaVersion: b.schemaVersion,
		objects.FieldKeyNamespaceID:   pkgctx.SystemAccountID,
	}

	fields := spec.ResolvedFields
	if len(fields) == 0 {
		fields = spec.Fields
	}
	fieldNames := make([]string, 0, len(fields))
	for fieldName := range fields {
		fieldNames = append(fieldNames, fieldName)
	}
	sort.Strings(fieldNames)

	for _, fieldName := range fieldNames {
		raw := fields[fieldName]
		fieldDef, ok := raw.(map[string]any)
		if !ok || !fieldIsRequired(fieldDef) {
			continue
		}
		if _, exists := out[fieldName]; exists {
			continue
		}
		out[fieldName] = placeholderValueForField(fieldName, fieldDef)
	}

	return out, nil
}

// BuildRequiredTemplateYAML marshals the required-field template as YAML text.
func (b *BaseInstanceBuilder) BuildRequiredTemplateYAML() (string, error) {
	templateMap, err := b.BuildRequiredTemplate()
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(templateMap)
	if err != nil {
		return "", errfmt.Newf("marshal template yaml").Wrap(err)
	}
	return string(data), nil
}

// BuildRequiredTemplateJSON marshals the required-field template as JSON text.
func (b *BaseInstanceBuilder) BuildRequiredTemplateJSON() (string, error) {
	templateMap, err := b.BuildRequiredTemplate()
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(templateMap, "", "  ")
	if err != nil {
		return "", errfmt.Newf("marshal template json").Wrap(err)
	}
	return string(data), nil
}

// Build creates a new instance map from the builder (thread-safe).
func (b *BaseInstanceBuilder) Build() (map[string]any, error) {
	var fieldsCopy map[string]any
	err := concurrency.RunInLock(&b.mu, func() error {
		if id, ok := b.fields[objects.FieldKeyID].(string); !ok || id == emptyValue {
			return errfmt.Errorf("id is required")
		}
		fieldsCopy = make(map[string]any)
		maps.Copy(fieldsCopy, b.fields)
		b.fields = make(map[string]any) // reset fields for next build
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Build instance from copy (no lock needed; b.kind etc. are immutable)
	instance := make(map[string]any)
	maps.Copy(instance, fieldsCopy)
	instance[objects.FieldKeyKind] = b.kind
	instance[objects.FieldKeySchemaVersion] = b.schemaVersion

	now := zqktime.NowRFC3339UTC()
	if _, ok := instance[objects.FieldKeyCreatedAt]; !ok {
		instance[objects.FieldKeyCreatedAt] = now
	}
	if _, ok := instance[objects.FieldKeyUpdatedAt]; !ok {
		instance[objects.FieldKeyUpdatedAt] = now
	}
	if _, ok := instance[objects.FieldKeyCreatedBy]; !ok {
		instance[objects.FieldKeyCreatedBy] = pkgctx.SystemAccountID
	}
	if _, ok := instance[objects.FieldKeyUpdatedBy]; !ok {
		instance[objects.FieldKeyUpdatedBy] = pkgctx.SystemAccountID
	}
	b.applyKindDefaults(instance)

	return instance, nil
}

// applyKindDefaults applies default values based on object kind
// This allows different kinds to have different default behaviors
func (b *BaseInstanceBuilder) applyKindDefaults(instance map[string]any) {
	// Metric kinds get default namespace, origin, and status
	if strings.HasSuffix(b.kind, "_metric") || b.kind == objects.KindBaseMetric {
		// Set default namespace if not set
		if _, ok := instance[objects.FieldKeyNamespaceID]; !ok {
			instance[objects.FieldKeyNamespaceID] = metricDefaultNamespaceID
		}
		// Set default origin_project if not set
		if _, ok := instance[objects.FieldKeyOriginProject]; !ok {
			instance[objects.FieldKeyOriginProject] = metricDefaultOriginValue
		}
		// Set default origin_system if not set
		if _, ok := instance[objects.FieldKeyOriginSystem]; !ok {
			instance[objects.FieldKeyOriginSystem] = metricDefaultOriginValue
		}
		// Set default status for metrics if not set.
		// Use a lifecycle-valid generic status ("implemented") so all metric kinds
		// share a safe default. Kinds that need a more specific status (e.g. "completed")
		// must call Status explicitly on their builders.
		if _, ok := instance[objects.FieldKeyStatus]; !ok {
			instance[objects.FieldKeyStatus] = metricDefaultStatus
		}
	}
}

// LoadFromYAML loads an instance from a YAML file
func (b *BaseInstanceBuilder) LoadFromYAML(yamlPath string) (map[string]any, error) {
	data, err := fileutil.ReadFile(yamlPath)
	if err != nil {
		return nil, errfmt.Newf("failed to read YAML file").Wrap(err)
	}

	var instance map[string]any
	if err := yaml.Unmarshal(data, &instance); err != nil {
		return nil, errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	return instance, nil
}

// WriteToYAML writes an instance to a YAML file
func (b *BaseInstanceBuilder) WriteToYAML(instance map[string]any, yamlPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(yamlPath)
	if err := fileutil.MkdirAll(dir, baseBuilderDirectoryPerm); err != nil {
		return errfmt.Newf("failed to create directory").Wrap(err)
	}

	data, err := yaml.Marshal(instance)
	if err != nil {
		return errfmt.Newf("failed to marshal YAML").Wrap(err)
	}

	if err := fileutil.WriteFile(yamlPath, data, baseBuilderFilePerm); err != nil {
		return errfmt.Newf("failed to write YAML file").Wrap(err)
	}

	return nil
}

// WriteToSequence writes an instance to a sequence file (compact format: values in order)
func (b *BaseInstanceBuilder) WriteToSequence(instance map[string]any, sequencePath string) error {
	// Ensure directory exists
	dir := filepath.Dir(sequencePath)
	if err := fileutil.MkdirAll(dir, baseBuilderDirectoryPerm); err != nil {
		return errfmt.Newf("failed to create directory").Wrap(err)
	}

	// Build sequence line: values in fieldOrder, pipe-delimited
	var values []string
	for _, fieldName := range b.fieldOrder {
		value := ""
		if v, ok := instance[fieldName]; ok {
			// Convert value to string and escape (empty strings are valid)
			value = fmt.Sprintf("%v", v)
			value = escapeSequenceValue(value)
		}
		values = append(values, value)
	}

	line := strings.Join(values, "|") + "\n"

	if err := fileutil.WriteFile(sequencePath, []byte(line), baseBuilderFilePerm); err != nil {
		return errfmt.Newf("failed to write sequence file").Wrap(err)
	}

	return nil
}

// LoadFromSequence loads an instance from a sequence file (compact format: values in order)
func (b *BaseInstanceBuilder) LoadFromSequence(sequencePath string) (map[string]any, error) {
	data, err := fileutil.ReadFile(sequencePath)
	if err != nil {
		return nil, errfmt.Newf("failed to read sequence file").Wrap(err)
	}

	line := strings.TrimSpace(string(data))
	values := splitSequenceLine(line)

	// Map values to fields using fieldOrder
	instance := make(map[string]any)
	for i, fieldName := range b.fieldOrder {
		if i < len(values) {
			value := unescapeSequenceValue(values[i])
			// Always store the value, even if empty (empty strings are valid)
			instance[fieldName] = value
		}
	}

	return instance, nil
}

// escapeSequenceValue escapes special characters for sequence format
// Escapes: | -> \|, \ -> \\, \n -> \n, \r -> \r
func escapeSequenceValue(value string) string {
	var result strings.Builder
	for _, r := range value {
		switch r {
		case '|':
			result.WriteString("\\|")
		case '\\':
			result.WriteString("\\\\")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		default:
			result.WriteRune(r)
		}
	}
	return result.String()
}

// unescapeSequenceValue unescapes special characters from sequence format
func unescapeSequenceValue(value string) string {
	var result strings.Builder
	escaped := false
	for i, r := range value {
		if escaped {
			switch r {
			case '|':
				result.WriteRune('|')
			case '\\':
				result.WriteRune('\\')
			case 'n':
				result.WriteRune('\n')
			case 'r':
				result.WriteRune('\r')
			default:
				// Unknown escape sequence, include both backslash and char
				result.WriteRune('\\')
				result.WriteRune(r)
			}
			escaped = false
			continue
		}

		if r == '\\' {
			escaped = true
			// Check if this is the last character (invalid escape)
			if i == len(value)-1 {
				result.WriteRune('\\')
			}
			continue
		}

		result.WriteRune(r)
	}
	return result.String()
}

// splitSequenceLine splits a sequence line by pipe delimiter, respecting escaped pipes
// NOTE: This parsing logic handles basic escaping but may need enhancement for edge cases.
// Future consideration: If handling weird characters becomes onerous, consider:
//   - Configuration options for delimiter/escaping strategy
//   - Profile-based handling (strict vs. lenient modes)
//   - Alternative formats (base64-encoded values, length-prefixed, etc.)
func splitSequenceLine(line string) []string {
	var values []string
	var current strings.Builder
	escaped := false

	for i, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			escaped = true
			// Only skip backslash if next char is pipe (for escaping)
			if i+1 < len(line) && line[i+1] == '|' {
				continue // Skip backslash, next iteration will handle the escaped pipe
			}
			current.WriteRune(r)
		} else if r == '|' {
			values = append(values, current.String())
			current.Reset()
		} else {
			current.WriteRune(r)
		}
	}

	// Add the last value
	values = append(values, current.String())
	return values
}

func fieldIsRequired(fieldDef map[string]any) bool {
	validation, ok := fieldDef[baseBuilderFieldValidation].(map[string]any)
	if !ok {
		return false
	}
	required, ok := validation[baseBuilderFieldRequired]
	if !ok {
		return false
	}
	switch v := required.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func placeholderValueForField(fieldName string, fieldDef map[string]any) any {
	fieldType, _ := fieldDef[objects.FieldKeyType].(string)
	fieldType = strings.ToLower(fieldType)
	validation, _ := fieldDef[baseBuilderFieldValidation].(map[string]any)

	switch fieldType {
	case fieldTypeEnum:
		if enumRaw, ok := validation[baseBuilderFieldEnum].([]any); ok {
			for _, v := range enumRaw {
				if s, ok := v.(string); ok && s != emptyValue {
					return s
				}
			}
		}
		return templateLoremIpsum
	case fieldTypeInteger, fieldTypeInt:
		return 0
	case fieldTypeNumber, fieldTypeFloat:
		return 0.0
	case fieldTypeBoolean, fieldTypeBool:
		return false
	case fieldTypeArray, fieldTypeList:
		return []any{templateLoremIpsum}
	case fieldTypeObject, fieldTypeMap:
		return map[string]any{"example_key": templateLoremIpsum}
	case fieldTypeTimestamp, fieldTypeDateTime:
		return templatePlaceholderTime
	case fieldTypeID, fieldTypeIdentifier:
		return templatePlaceholderID
	default:
	}

	if IsChronoFieldName(fieldName) {
		return templatePlaceholderTime
	}
	if IsIDFieldName(fieldName) {
		return templatePlaceholderID
	}
	return templateLoremIpsum
}

// IsChronoFieldName returns true when a field name represents date/time semantics.
func IsChronoFieldName(fieldName string) bool {
	for _, token := range chronoFieldNameIndicators {
		if strings.Contains(fieldName, token) {
			return true
		}
	}
	return false
}

// IsIDFieldName returns true when a field name represents ID semantics.
func IsIDFieldName(fieldName string) bool {
	return fieldName == objects.FieldKeyID || strings.HasSuffix(fieldName, "_id")
}

func baseBuilderSpecFileNameForKind(kind string) string {
	return kind + baseBuilderSpecFileExtYAML
}
