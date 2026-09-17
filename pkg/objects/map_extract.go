package objects

// GetString safely extracts a string value from a map[string]any,
// returning the empty string if the key does not exist or the value is not a string.
func GetString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}
