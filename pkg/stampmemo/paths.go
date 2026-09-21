package stampmemo

// Combine returns the newest stamp. Missing paths (0) do not win.
func Combine(stamps ...Stamp) Stamp {
	var max Stamp
	for _, s := range stamps {
		if s > max {
			max = s
		}
	}
	return max
}

// OfAll is Combine(Of(path)...) for a candidate list.
func OfAll(paths ...string) Stamp {
	stamps := make([]Stamp, len(paths))
	for i, p := range paths {
		stamps[i] = Of(p)
	}
	return Combine(stamps...)
}

// FirstExisting returns the first path whose stamp is non-zero.
func FirstExisting(paths []string) string {
	for _, p := range paths {
		if p != "" && Of(p) != 0 {
			return p
		}
	}
	return ""
}
