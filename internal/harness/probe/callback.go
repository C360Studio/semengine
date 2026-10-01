package probe

import (
	"context"
	"sync"
)

// Callback is a callback body a test holds open to observe an owner while work is in flight: it
// signals entry, blocks until released, records whether its context was still live at release,
// and signals when it has returned.
//
// It deliberately keeps no context: it records ctx.Err() at the moment of release instead. Context
// cancellation is monotonic, so a nil error at release proves the context was live for the whole
// time the callback was blocked.
type Callback struct {
	entered, release, joined         chan struct{}
	enterOnce, releaseOnce, joinOnce sync.Once
	mu                               sync.Mutex
	errAtRelease                     error
	recorded                         bool
}

// NewCallback returns a callback that has not been entered or released.
func NewCallback() *Callback {
	return &Callback{entered: make(chan struct{}), release: make(chan struct{}), joined: make(chan struct{})}
}

// Block is the callback body. It closes Entered, waits for Release, records ctx.Err(), and closes
// Joined as it returns. It does not return early when ctx ends: a callback that ignores its context
// is exactly the shape an owner's drain must wait for. Later calls behave the same but record and
// signal nothing new.
func (c *Callback) Block(ctx context.Context) {
	c.enterOnce.Do(func() { close(c.entered) })
	defer c.joinOnce.Do(func() { close(c.joined) })
	<-c.release
	c.mu.Lock()
	if !c.recorded {
		c.errAtRelease, c.recorded = ctx.Err(), true
	}
	c.mu.Unlock()
}

// Entered is closed when Block is first called.
func (c *Callback) Entered() <-chan struct{} { return c.entered }

// Release lets every current and future Block return. It is idempotent, so a test may both
// release explicitly and defer a release for its failure paths.
func (c *Callback) Release() { c.releaseOnce.Do(func() { close(c.release) }) }

// Joined is closed when the first Block has returned.
func (c *Callback) Joined() <-chan struct{} { return c.joined }

// ContextErrAtRelease is ctx.Err() as the first Block saw it when released; nil means the
// callback's context stayed live throughout. Before the first release it is nil.
func (c *Callback) ContextErrAtRelease() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.errAtRelease
}
