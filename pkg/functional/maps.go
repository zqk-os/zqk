package functional

// MapGetPtr looks up a key in a map of pointers and returns the value.
// It returns nil if the key is not present or the value is nil.
func MapGetPtr[K comparable, V any](m map[K]*V, key K) *V {
	if m == nil {
		return nil
	}
	return m[key]
}

// MapHasPtr checks if a key is present in a map of pointers AND the value is not nil.
func MapHasPtr[K comparable, V any](m map[K]*V, key K) bool {
	return MapGetPtr(m, key) != nil
}

// MapGetOrDefault returns the value for the key if it exists, otherwise the default value.
func MapGetOrDefault[K comparable, V any](m map[K]V, key K, def V) V {
	if m == nil {
		return def
	}
	if v, ok := m[key]; ok {
		return v
	}
	return def
}

// MapKeys returns a slice of keys from the map.
func MapKeys[K comparable, V any](m map[K]V) []K {
	if m == nil {
		return nil
	}
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// MapValues returns a slice of values from the map.
func MapValues[K comparable, V any](m map[K]V) []V {
	if m == nil {
		return nil
	}
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

// FilterMap returns a new map containing all entries of m that satisfy the predicate.
// Returns nil if m is nil.
func FilterMap[K comparable, V any](m map[K]V, predicate func(K, V) bool) map[K]V {
	if m == nil {
		return nil
	}
	res := make(map[K]V)
	for k, v := range m {
		if predicate(k, v) {
			res[k] = v
		}
	}
	return res
}

// TransformMap applies the mapper function fn to each value in m and returns a new map.
// Returns nil if m is nil.
func TransformMap[K comparable, V, U any](m map[K]V, fn func(V) U) map[K]U {
	if m == nil {
		return nil
	}
	res := make(map[K]U, len(m))
	for k, v := range m {
		res[k] = fn(v)
	}
	return res
}

// MergeMaps merges multiple maps from left to right into a new map.
// Keys in later maps overwrite matching keys from earlier maps.
// Returns nil if no maps are provided or all provided maps are nil.
func MergeMaps[K comparable, V any](maps ...map[K]V) map[K]V {
	if len(maps) == 0 {
		return nil
	}
	hasNonNull := false
	for _, m := range maps {
		if m != nil {
			hasNonNull = true
			break
		}
	}
	if !hasNonNull {
		return nil
	}
	res := make(map[K]V)
	for _, m := range maps {
		for k, v := range m {
			res[k] = v
		}
	}
	return res
}

// InvertMap inverts the key-value pairs of a map so values become keys and keys become values.
// If multiple keys share the same value, the last one visited will overwrite previous entries.
// Returns nil if m is nil.
func InvertMap[K comparable, V comparable](m map[K]V) map[V]K {
	if m == nil {
		return nil
	}
	res := make(map[V]K, len(m))
	for k, v := range m {
		res[v] = k
	}
	return res
}

// Pick returns a new map containing only the specified keys from m.
// If a key does not exist in m, it is ignored. Returns nil if m is nil.
func Pick[K comparable, V any](m map[K]V, keys ...K) map[K]V {
	if m == nil {
		return nil
	}
	res := make(map[K]V)
	for _, k := range keys {
		if v, ok := m[k]; ok {
			res[k] = v
		}
	}
	return res
}

// Omit returns a new map containing all entries from m except those with the specified keys.
// Returns nil if m is nil.
func Omit[K comparable, V any](m map[K]V, keys ...K) map[K]V {
	if m == nil {
		return nil
	}
	omitSet := make(map[K]struct{}, len(keys))
	for _, k := range keys {
		omitSet[k] = struct{}{}
	}
	res := make(map[K]V)
	for k, v := range m {
		if _, omitted := omitSet[k]; !omitted {
			res[k] = v
		}
	}
	return res
}
