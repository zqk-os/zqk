package when

// ChainT is a generic version of Chain that returns a value.
type ChainT[T any] struct {
	clauses []clauseT[T]
}

type clauseT[T any] struct {
	cond   func() bool
	action func() T
}

// Result starts a generic chain that returns a value of type T.
func Result[T any]() *ChainT[T] {
	return &ChainT[T]{}
}

// When adds the first condition to the generic chain.
func (c *ChainT[T]) When(cond func() bool) *BuilderT[T] {
	return &BuilderT[T]{chain: c, cond: cond}
}

// BuilderT is the intermediate state for generic chains.
type BuilderT[T any] struct {
	chain *ChainT[T]
	cond  func() bool
}

// Then adds the action for the current condition.
func (b *BuilderT[T]) Then(fn func() T) *ChainT[T] {
	b.chain.clauses = append(b.chain.clauses, clauseT[T]{cond: b.cond, action: fn})
	return b.chain
}

// OrElseWhen adds another condition to the generic chain.
func (c *ChainT[T]) OrElseWhen(cond func() bool) *BuilderT[T] {
	return &BuilderT[T]{chain: c, cond: cond}
}

// OrElse adds a final fallback action.
func (c *ChainT[T]) OrElse(fn func() T) *ChainT[T] {
	c.clauses = append(c.clauses, clauseT[T]{cond: func() bool { return true }, action: fn})
	return c
}

// Run evaluates the chain and returns the result.
func (c *ChainT[T]) Run() T {
	for _, cl := range c.clauses {
		if cl.cond() {
			return cl.action()
		}
	}
	var zero T
	return zero
}
