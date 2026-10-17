package natsclient

import (
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/stretchr/testify/assert"
)

// T7, design "Tests that replace the probes" and D5. natsclient's two client
// sentinels are transient by errs.IsTransient and by errs.Classify, which on
// main disagree about them (P10, #167), and each matches the pkg/errs sentinel
// for its fact.
func TestClientSentinelsAreTransient(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		matches error
	}{
		{"ErrNotConnected", ErrNotConnected, errs.ErrConnectionLost},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, errs.IsTransient(tc.err), "errs.IsTransient(%v)", tc.err)
			assert.Equal(t, errs.ErrorTransient, errs.Classify(tc.err), "errs.Classify(%v)", tc.err)
			assert.ErrorIs(t, tc.err, tc.matches)
		})
	}
}
