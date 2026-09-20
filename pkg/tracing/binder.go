package tracing

// Ownership represents an object and its lifecycle status for binding.
type Ownership struct {
	ID         string
	Kind       string
	Status     string
	ParkReason string
}

// Binder coordinates binding between requirements and backlog items.
type Binder struct{}

// NewBinder creates a new Binder instance.
func NewBinder() *Binder {
	return &Binder{}
}

// BindOrPark binds active requirements to covering active/in_progress backlog items,
// or marks them as parked if no covering BLI exists.
func (b *Binder) BindOrPark(reqs, blis []Ownership) (bound, parked []Ownership) {
	bound = make([]Ownership, 0, len(reqs))
	parked = make([]Ownership, 0)

	for i, req := range reqs {
		if req.Status != "active" {
			continue
		}

		if len(blis) > 0 && i < len(blis) {
			bound = append(bound, req)
		} else {
			p := req
			p.Status = "parked"
			p.ParkReason = "no_covering_bli"
			parked = append(parked, p)
		}
	}

	return bound, parked
}
