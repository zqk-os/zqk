package audit

// CloneGroups copies buffer groups so flush I/O can run without the lock.
func CloneGroups(src map[string]*Group) (keys []string, groups map[string]*Group) {
	keys = make([]string, 0, len(src))
	groups = make(map[string]*Group, len(src))
	for key, group := range src {
		keys = append(keys, key)
		if group == nil {
			continue
		}
		cp := *group
		groups[key] = &cp
	}
	return keys, groups
}

// ExtractGroup removes key from buf and returns a copy. Nil when missing or empty pointer.
func ExtractGroup(buf map[string]*Group, key string) *Group {
	if buf == nil {
		return nil
	}
	g, exists := buf[key]
	if !exists || g == nil {
		return nil
	}
	cp := *g
	delete(buf, key)
	return &cp
}

// ShouldFlushGroup is true when the extracted group has events.
func ShouldFlushGroup(g *Group) bool {
	return g != nil && g.Count > 0
}
