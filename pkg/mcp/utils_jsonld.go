package mcp

const (
	flagTypeBool        = "bool"
	flagTypeInt         = "int"
	flagTypeInt8        = "int8"
	flagTypeInt16       = "int16"
	flagTypeInt32       = "int32"
	flagTypeInt64       = "int64"
	flagTypeUint        = "uint"
	flagTypeUint8       = "uint8"
	flagTypeUint16      = "uint16"
	flagTypeUint32      = "uint32"
	flagTypeUint64      = "uint64"
	flagTypeFloat32     = "float32"
	flagTypeFloat64     = "float64"
	flagTypeString      = "string"
	flagTypeStringSlice = "stringSlice"
	flagTypeStringArray = "stringArray"
	flagTypeText        = "text"
	flagTypeUUID        = "uuid"
	flagTypeID          = "id"
	flagTypeInteger     = "integer"
	flagTypeNumber      = "number"
	flagTypeFloat       = "float"
	flagTypeBoolean     = "boolean"
	flagTypeList        = "list"
	flagTypeArray       = "array"
	flagTypeObject      = "object"
	flagTypeMap         = "map"
	flagTypeEnum        = "enum"
	flagTypeReference   = "reference"
	flagValueTrue       = "true"
	xsdString           = "xsd:string"
	xsdBoolean          = "xsd:boolean"
	xsdInteger          = "xsd:integer"
	xsdDouble           = "xsd:double"
	xsdArray            = "xsd:array"
	xsdObject           = "xsd:object"
	zqkEnum             = "zqk:Enum"
	zqkReference        = "zqk:Reference"
)

// MapFlagTypeToJSONLDType maps pflag types to JSON-LD/XSD types.
// Used when converting CLI command flags to JSON-LD schema properties.
// Returns XSD type string (e.g., "xsd:boolean", "xsd:string").
func MapFlagTypeToJSONLDType(flagType string) string {
	switch flagType {
	case flagTypeBool:
		return xsdBoolean
	case flagTypeInt, flagTypeInt8, flagTypeInt16, flagTypeInt32, flagTypeInt64, flagTypeUint, flagTypeUint8, flagTypeUint16, flagTypeUint32, flagTypeUint64:
		return xsdInteger
	case flagTypeFloat32, flagTypeFloat64:
		return xsdDouble
	case flagTypeString:
		return xsdString
	case flagTypeStringSlice, flagTypeStringArray:
		return xsdArray
	default:
		return xsdString
	}
}

// ParseDefaultValue parses a default value string to appropriate type based on flag type.
// Used when converting CLI command flags to JSON-LD schema properties.
// Returns typed value (bool for boolean flags, string for others to avoid JSON type issues).
func ParseDefaultValue(defValue, flagType string) any {
	switch flagType {
	case flagTypeBool:
		return defValue == flagValueTrue
	case flagTypeInt, flagTypeInt8, flagTypeInt16, flagTypeInt32, flagTypeInt64, flagTypeUint, flagTypeUint8, flagTypeUint16, flagTypeUint32, flagTypeUint64:
		// Return as string to avoid type issues (JSON will handle it)
		return defValue
	case flagTypeFloat32, flagTypeFloat64:
		return defValue
	default:
		return defValue
	}
}

// MapFieldTypeToXSD maps ZQK field types to XSD types.
// Used when converting object schema fields to JSON-LD format.
// Returns XSD type string or custom type (e.g., "zqk:Enum", "zqk:Reference").
func MapFieldTypeToXSD(fieldType string) string {
	switch fieldType {
	case flagTypeString, flagTypeText, flagTypeUUID, flagTypeID:
		return xsdString
	case flagTypeInteger, flagTypeInt:
		return xsdInteger
	case flagTypeNumber, flagTypeFloat:
		return xsdDouble
	case flagTypeBoolean, flagTypeBool:
		return xsdBoolean
	case flagTypeList, flagTypeArray:
		return xsdArray
	case flagTypeObject, flagTypeMap:
		return xsdObject
	case flagTypeEnum:
		return zqkEnum
	case flagTypeReference:
		return zqkReference
	default:
		return xsdString
	}
}

// MapFieldTypeToElicitationType maps field types to elicitation parameter types.
// Used when converting object fields to interactive elicitation parameters.
// Returns elicitation type string.
func MapFieldTypeToElicitationType(fieldType string) string {
	switch fieldType {
	case flagTypeString, flagTypeText, flagTypeUUID, flagTypeID:
		return flagTypeString
	case flagTypeInteger, flagTypeInt:
		return flagTypeNumber
	case flagTypeNumber, flagTypeFloat:
		return flagTypeNumber
	case flagTypeBoolean, flagTypeBool:
		return flagTypeBoolean
	case flagTypeList, flagTypeArray:
		return flagTypeArray
	case flagTypeObject, flagTypeMap:
		return flagTypeObject
	case flagTypeEnum:
		return flagTypeEnum
	case flagTypeReference:
		return flagTypeReference
	default:
		return flagTypeString
	}
}
