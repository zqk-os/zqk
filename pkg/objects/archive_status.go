package objects

// ArchiveStatusForKind returns the lifecycle status retention (and similar jobs) should
// write when "archiving" objects of kind. Prefers a status with archive:true whose value
// is ObjectStatusArchived; otherwise the first archive-flagged status.
//
// ok is false when the kind has no archive status — callers must not invent literal
// "archived" (question uses resolved/deferred; writing archived caused system-check churn).
// TRACK: REDACTED — remove when: retention never writes status
// without resolving a lifecycle-valid archive status for the kind.
func ArchiveStatusForKind(kind string) (status string, ok bool) {
	return GetGlobalLifecycleLoader().ArchiveStatusForKind(kind)
}

// ArchiveStatusForKind resolves the archive status for kind from lifecycle YAML.
func (ll *LifecycleLoader) ArchiveStatusForKind(kind string) (status string, ok bool) {
	if ll == nil || kind == "" {
		return "", false
	}
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil || lifecycle == nil {
		return "", false
	}
	var firstArchive string
	for _, s := range lifecycle.Statuses {
		if !s.Archive || s.Value == "" {
			continue
		}
		if s.Value == ObjectStatusArchived {
			return ObjectStatusArchived, true
		}
		if firstArchive == "" {
			firstArchive = s.Value
		}
	}
	if firstArchive != "" {
		return firstArchive, true
	}
	return "", false
}
