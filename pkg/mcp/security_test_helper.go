package mcp

// detectPermissionCycle detects cycles in permission dependencies using DFS
func detectPermissionCycle(perm string, deps map[string][]string, visited map[string]bool) bool {
	if visited[perm] {
		return true // Cycle detected
	}

	visited[perm] = true
	defer func() { visited[perm] = false }() // Backtrack

	for _, dep := range deps[perm] {
		if detectPermissionCycle(dep, deps, visited) {
			return true
		}
	}

	return false
}
