package when

// clause holds a condition and the action to run when it is true.
type clause struct {
	cond   func() bool
	action func()
	flow   FlowSignal
}

// Chain runs the first branch whose condition is true.
// Build with When(cond).Then(fn).OrElseWhen(cond).Then(fn).OrElse(fn).Run().
type Chain struct {
	clauses []clause
}

// FlowSignal describes optional control intent produced by Execute.
// Callers should handle this explicitly (e.g. switch for return/continue/break).
type FlowSignal int

const (
	FlowNone FlowSignal = iota
	FlowReturn
	FlowContinue
	FlowBreak
)

// When starts a chain: when cond() is true, the next Then(fn) will run that fn.
// Conditions are evaluated at Run() time, so cond can close over current variables.
//
// Example:
//
//	when.When(func() bool { return err != nil }).Then(func() { log.Warn(...); x = default }).
//		OrElseWhen(func() bool { return x == nil }).Then(func() { x = default }).
//		Run()
func When(cond func() bool) *Builder {
	return &Builder{chain: &Chain{}, cond: cond}
}

// Builder is the intermediate state after When(cond) or OrElseWhen(cond); call Then(fn) next.
type Builder struct {
	chain *Chain
	cond  func() bool
}

// Then adds this branch to the chain and returns the chain for further OrElseWhen/OrElse.
func (b *Builder) Then(fn func()) *Chain {
	b.chain.clauses = append(b.chain.clauses, clause{cond: b.cond, action: fn, flow: FlowNone})
	return b.chain
}

// ThenDo is an alias for Then for fluent readability.
func (b *Builder) ThenDo(fn func()) *Chain {
	return b.Then(fn)
}

// OrElseWhen adds another condition; the next Then(fn) will run when this condition is true.
func (c *Chain) OrElseWhen(cond func() bool) *Builder {
	return &Builder{chain: c, cond: cond}
}

// OrElse adds a final branch that always runs if no prior condition matched.
// Returns the chain so you can call Run().
func (c *Chain) OrElse(fn func()) *Chain {
	c.clauses = append(c.clauses, clause{cond: func() bool { return true }, action: fn, flow: FlowNone})
	return c
}

// Run evaluates conditions in order and runs the first matching action, then returns.
func (c *Chain) Run() {
	for _, cl := range c.clauses {
		if cl.cond() {
			cl.action()
			return
		}
	}
}

// Execute runs like Run but returns a flow signal for explicit caller control.
func (c *Chain) Execute() FlowSignal {
	for _, cl := range c.clauses {
		if cl.cond() {
			cl.action()
			return cl.flow
		}
	}
	return FlowNone
}

// AndReturn marks the last matching branch as "caller should return".
func (c *Chain) AndReturn() *Chain {
	if n := len(c.clauses); n > 0 {
		c.clauses[n-1].flow = FlowReturn
	}
	return c
}

// AndContinue marks the last matching branch as "caller should continue".
func (c *Chain) AndContinue() *Chain {
	if n := len(c.clauses); n > 0 {
		c.clauses[n-1].flow = FlowContinue
	}
	return c
}

// AndBreak marks the last matching branch as "caller should break".
func (c *Chain) AndBreak() *Chain {
	if n := len(c.clauses); n > 0 {
		c.clauses[n-1].flow = FlowBreak
	}
	return c
}

// WhenNilOrEmpty starts a chain that runs when v is nil or empty.
func WhenNilOrEmpty(v any) *Builder {
	return When(func() bool { return IsNilOrEmpty(v) })
}

// WhenNotNilOrEmpty starts a chain that runs when v is not nil and not empty.
func WhenNotNilOrEmpty(v any) *Builder {
	return When(func() bool { return !IsNilOrEmpty(v) })
}

// WhenBlank starts a chain that runs when s is empty or whitespace-only.
func WhenBlank(s string) *Builder {
	return When(func() bool { return IsBlank(s) })
}

// WhenNotBlank starts a chain that runs when s contains non-whitespace content.
func WhenNotBlank(s string) *Builder {
	return When(func() bool { return !IsBlank(s) })
}
