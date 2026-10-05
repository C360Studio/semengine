package natsclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReviewerExportedNATSNilContextRefuses(t *testing.T) {
	c, err := NewClient(embeddedJetStreamURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)
	spec := validNoLifecycleSpec()
	_, err = EnsureFrameworkBucket(t.Context(), c, spec)
	require.NoError(t, err)
	_, err = OpenFrameworkBucket(t.Context(), c, spec)
	require.NoError(t, err)
	_, err = c.SubscribeForRequests(t.Context(), "review.nil", func(context.Context, []byte) ([]byte, error) { return []byte("ok"), nil })
	require.NoError(t, err)
	require.NoError(t, c.GetConnection().FlushTimeout(10*time.Second))
	data, err := c.Request(t.Context(), "review.nil", nil, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, "ok", string(data))
	for _, tc := range []struct {
		name string
		call func(context.Context) error
	}{
		{"OpenFrameworkBucket", func(ctx context.Context) error { _, err := OpenFrameworkBucket(ctx, c, spec); return err }},
		{"Request", func(ctx context.Context) error {
			_, err := c.Request(ctx, "review.nil", nil, 10*time.Second)
			return err
		}},
		{"RequestWithHeaders", func(ctx context.Context) error {
			_, err := c.RequestWithHeaders(ctx, "review.nil", nil, nil, 10*time.Second)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("exported nil context boundary panicked: %v", p)
				}
			}()
			var nilCtx context.Context
			err := tc.call(nilCtx)
			t.Logf("nil context returned %v", err)
			if err == nil {
				t.Error("nil context accepted")
			}
		})
	}
}
