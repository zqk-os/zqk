package objects

// StringField extracts a string value from a map, returning empty string if missing or wrong type.
func StringField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	v, _ := obj[key].(string)
	return v
}
