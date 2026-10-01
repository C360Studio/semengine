package probe

import (
	"context"
	"sync"
)

// ObservedContext is a context that signals the first time anyone calls Done on it, and can hold
// that caller until the test lets it proceed. A test passes it as an owner's Stop context and waits
// on Observed to know Stop has reached the point where it waits on its caller's authority, without
// sleeping.
//
// It is itself a context.Context derived from its parent, so it holds the parent the way the
// standard library's derived contexts do; contract test T-B2 exempts this one type by name.
type ObservedContext struct {
	context.Context
	observed chan struct{}
	proceed  <-chan struct{}
	once     sync.Once
}

// Observe returns a context derived from parent whose first Done call closes Observed.
func Observe(parent context.Context) *ObservedContext {
	return ObserveAndHold(parent, nil)
}

// ObserveAndHold is Observe, except that the first Done call also blocks until proceed is closed.
// A nil proceed does not hold. Like context.WithCancel, it panics on a nil parent.
func ObserveAndHold(parent context.Context, proceed <-chan struct{}) *ObservedContext {
	if parent == nil {
		panic("probe: nil parent context")
	}
	return &ObservedContext{Context: parent, observed: make(chan struct{}), proceed: proceed}
}

// Observed is closed by the first Done call.
func (c *ObservedContext) Observed() <-chan struct{} { return c.observed }

// Done returns the parent's Done channel, signalling (and, if asked, holding) the first caller.
func (c *ObservedContext) Done() <-chan struct{} {
	c.once.Do(func() {
		close(c.observed)
		if c.proceed != nil {
			<-c.proceed
		}
	})
	return c.Context.Done()
}
