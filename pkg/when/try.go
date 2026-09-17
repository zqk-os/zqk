package when

// Try captures a (value, error) pair for fluent handling without swallowing errors.
//
//	v, err := when.Try(cmd.Flags().GetString("channel")).
//		UnlessErr(func(e error) error { return errfmt.Newf("channel flag").Wrap(e) }).
//		Get()
//
// Chain shape aligns with IfTrue(cond).Then(...).UnlessErr(...).OrElse(...):
// Then runs only when err is still nil; UnlessErr annotates when err is set;
// OrElse supplies a fallback value only when err != nil (error is preserved —
// OrElse does not clear err; use ClearErr only when a fallback intentionally recovers).
//
// Constructors always return a non-nil carrier; chain methods assume that and do not
// re-check the receiver for nil (same posture as when.When / Chain).
func Try[T any](v T, err error) *TryResult[T] {
	return &TryResult[T]{val: v, err: err}
}

// TryResult is a fluent (value, error) carrier.
type TryResult[T any] struct {
	val T
	err error
}

// Then transforms the value when there is no error yet.
func (r *TryResult[T]) Then(fn func(T) T) *TryResult[T] {
	if r.err == nil {
		r.val = fn(r.val)
	}
	return r
}

// UnlessErr annotates or replaces the error when one is present. Does not clear it.
func (r *TryResult[T]) UnlessErr(fn func(error) error) *TryResult[T] {
	if r.err != nil {
		r.err = fn(r.err)
	}
	return r
}

// OrElse sets a fallback value when err != nil. The error remains set unless ClearErr follows.
func (r *TryResult[T]) OrElse(fn func() T) *TryResult[T] {
	if r.err != nil {
		r.val = fn()
	}
	return r
}

// ClearErr drops the error after an intentional recovery (rare; prefer returning err).
func (r *TryResult[T]) ClearErr() *TryResult[T] {
	r.err = nil
	return r
}

// Get returns the value and error (same contract as the original Try pair).
func (r *TryResult[T]) Get() (T, error) {
	return r.val, r.err
}

// Err returns the accumulated error.
func (r *TryResult[T]) Err() error {
	return r.err
}

// Val returns the value (may be zero/fallback when Err() != nil).
func (r *TryResult[T]) Val() T {
	return r.val
}

// IfTrue starts an error-aware branch: Then runs when cond is true and no prior err.
//
//	err := when.IfTrue(needWork).
//		Then(func() error { return doWork() }).
//		UnlessErr(func(e error) error { return errfmt.Newf("doWork").Wrap(e) }).
//		OrElse(func() error { return skipWork() }).
//		Err()
func IfTrue(cond bool) *ErrChain {
	return &ErrChain{skipThen: !cond}
}

// ErrChain is IfTrue(cond).Then(...).UnlessErr(...).OrElse(...).
type ErrChain struct {
	err      error
	skipThen bool
	didThen  bool
}

// Then runs fn when the opening condition was true and no error is set yet.
func (c *ErrChain) Then(fn func() error) *ErrChain {
	if c.err != nil || c.skipThen {
		return c
	}
	c.didThen = true
	c.err = fn()
	return c
}

// UnlessErr annotates the error when present.
func (c *ErrChain) UnlessErr(fn func(error) error) *ErrChain {
	if c.err != nil {
		c.err = fn(c.err)
	}
	return c
}

// OrElse runs when Then was skipped (condition false) and no error is set.
func (c *ErrChain) OrElse(fn func() error) *ErrChain {
	if c.err != nil || c.didThen {
		return c
	}
	c.err = fn()
	return c
}

// Err returns the chain error.
func (c *ErrChain) Err() error {
	return c.err
}
