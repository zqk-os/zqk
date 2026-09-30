package predicate

import (
	"testing"
)

func TestStem(t *testing.T) {
	tests := []struct {
		word string
		want string
	}{
		{"authentication", "authent"},
		{"authenticating", "authent"},
		{"authenticated", "authent"},
		{"connections", "connect"},
		{"connecting", "connect"},
		{"connected", "connect"},
		{"locks", "lock"},
		{"locking", "lock"},
		{"locked", "lock"},
		{"reallocations", "realloc"},
		{"reallocating", "realloc"},
		{"measurements", "measur"},
		{"measuring", "measur"},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := Stem(tt.word)
			if got != tt.want {
				t.Errorf("Stem(%q) = %q, want %q", tt.word, got, tt.want)
			}
		})
	}
}

func TestVerifyTitleBodyCohesion(t *testing.T) {
	body := `ReverseReferenceIndex maintains an in-memory cache of object dependencies.
Under high load, concurrent object deletions trigger a linear map scan and slice reallocation
while holding the global sync.RWMutex write lock. This causes lock contention and latency spikes.`

	// 1. Valid representative title containing salient keywords
	validTitle := "ReverseReferenceIndex lock contention and slice reallocation"
	ok, shared := VerifyTitleBodyCohesion(validTitle, body, 1)
	if !ok {
		t.Errorf("expected valid title to pass cohesion check, got false; shared: %v", shared)
	}
	if len(shared) == 0 {
		t.Errorf("expected shared stems, got none")
	}

	// 2. Another valid title with morphological variant ("locking" vs "lock")
	validTitleMorph := "Resolve locking latency in reverse reference index"
	ok, shared = VerifyTitleBodyCohesion(validTitleMorph, body, 1)
	if !ok {
		t.Errorf("expected morphological variant title to pass, got false; shared: %v", shared)
	}

	// 3. Vacuous generic title with zero overlap
	vacuousTitle := "Perform necessary system refactoring tasks"
	ok, shared = VerifyTitleBodyCohesion(vacuousTitle, body, 1)
	if ok {
		t.Errorf("expected vacuous title to fail cohesion check, but it passed with shared: %v", shared)
	}
	if len(shared) != 0 {
		t.Errorf("expected 0 shared stems for vacuous title, got %v", shared)
	}

	// 4. Another completely detached title
	detachedTitle := "Update user billing subscription profile"
	ok, shared = VerifyTitleBodyCohesion(detachedTitle, body, 1)
	if ok {
		t.Errorf("expected detached title to fail cohesion check, but it passed with shared: %v", shared)
	}
}

func TestCalculateTitleBodyOverlap(t *testing.T) {
	body := "Implement distributed lock fencing tokens to prevent split-brain execution across workers."
	title := "Fencing tokens for distributed lock"

	overlap, shared := CalculateTitleBodyOverlap(title, body, 10)
	if overlap <= 0 {
		t.Errorf("expected positive overlap, got %f", overlap)
	}
	if len(shared) < 2 {
		t.Errorf("expected at least 2 shared stems, got %d (%v)", len(shared), shared)
	}
}
