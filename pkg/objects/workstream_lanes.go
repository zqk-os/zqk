package objects

// IsWorkstreamLaneField reports whether name is the Gantt workstream bind
// (singular legacy workstream_ref or canonical workstream_refs).
func IsWorkstreamLaneField(name string) bool {
	return name == FieldKeyWorkstreamRef || name == FieldKeyWorkstreamRefs
}

// WorkstreamLaneIDs returns non-empty workstream IDs from singular workstream_ref
// and/or plural workstream_refs (deduped, first-seen order). Empty when neither is set.
func WorkstreamLaneIDs(obj map[string]any) []string {
	return KernelObjectRefIDs(obj, FieldKeyWorkstreamRefs)
}

// WorkstreamLaneContains reports whether want is a bound workstream id on obj.
func WorkstreamLaneContains(obj map[string]any, want string) bool {
	return KernelObjectRefContains(obj, FieldKeyWorkstreamRefs, want)
}

// WorkstreamLaneGroupKey is the --group-by key for either workstream field.
func WorkstreamLaneGroupKey(obj map[string]any) string {
	return KernelObjectRefGroupKey(obj, FieldKeyWorkstreamRefs)
}

// WorkstreamLaneIDs returns bound workstream ids from typed slices and raw aliases.
func (p *ParsedObject) WorkstreamLaneIDs() []string {
	return p.KernelObjectRefIDs(FieldKeyWorkstreamRefs)
}
