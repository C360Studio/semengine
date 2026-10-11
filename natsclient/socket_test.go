package natsclient

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// After a cut, a dial must not reach the network: a server that is listening
// would see a connection that the dialer then closes (design review r2-2, NIT).
// A second cut returns, so Connect's and Close's cuts can both reach one
// dialer.
func TestSocketDialerRefusesADialAfterTheCut(t *testing.T) {
	d := newSocketDialer(time.Second, func(*net.Dialer, string, string) (net.Conn, error) {
		t.Error("the dial reached the network after the cut")
		return nil, errors.New("unexpected dial")
	})
	d.cut()
	d.cut()

	_, err := d.Dial("tcp", "unused")
	require.ErrorIs(t, err, net.ErrClosed)
}
