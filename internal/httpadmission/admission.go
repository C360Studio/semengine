// Package httpadmission counts the requests an HTTP handler has admitted and not
// yet returned from, so a server's stop can wait for them after a forced close.
//
// net/http stops tracking a handler once its connection is closed: after
// http.Server.Close, a later Shutdown returns nil at once while the handler still
// runs (setup-04a-03-boot-service-manager design D2, probe C3 on #353). A server
// that must not report itself stopped while a handler runs wraps its handler with
// Requests.Wrap, closes admission when its stop begins, and waits with
// Requests.Wait. metric.Server and the service manager's listeners use it.
package httpadmission

import (
	"context"
	"net/http"
	"sync"
)

// Requests counts admitted requests. Closing admission first means that once it
// is closed and nothing runs, which Wait observes under the same lock, no request
// can start. The zero value is ready to use; a Requests must not be copied after
// first use.
type Requests struct {
	mu     sync.Mutex
	active int
	closed bool          // admission closed by CloseAdmission
	idle   chan struct{} // closed when active returns to zero; nil while none run
}

// Wrap returns a handler that admits each request through r before calling next.
// After CloseAdmission a request is refused with 503, the text refusal and
// "Connection: close", and next is not called.
func (r *Requests) Wrap(next http.Handler, refusal string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !r.begin() {
			// The stop has begun: the request is refused rather than started,
			// because the stop may already have observed that no request runs.
			w.Header().Set("Connection", "close")
			http.Error(w, refusal, http.StatusServiceUnavailable)
			return
		}
		defer r.end()
		next.ServeHTTP(w, req)
	})
}

// begin admits a request, or reports false once admission is closed.
func (r *Requests) begin() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}
	if r.active == 0 {
		r.idle = make(chan struct{})
	}
	r.active++
	return true
}

// CloseAdmission refuses every later request; requests already admitted run on.
func (r *Requests) CloseAdmission() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
}

func (r *Requests) end() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active--
	if r.active == 0 {
		close(r.idle)
		r.idle = nil
	}
}

// Pending returns how many admitted requests are still running.
func (r *Requests) Pending() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

// Wait returns nil once no admitted request is running, or ctx's error if ctx
// ends first.
func (r *Requests) Wait(ctx context.Context) error {
	for {
		r.mu.Lock()
		idle := r.idle
		r.mu.Unlock()
		if idle == nil {
			return nil
		}
		select {
		case <-idle:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
