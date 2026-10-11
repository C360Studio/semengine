package natsclient

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/pkg/errs"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func TestConnectRejectsCandidateWhenContextCanceledDuringDial(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)
	client.healthInterval = 0

	candidate := newConnectCandidate(t)
	dialEntered := make(chan struct{})
	releaseDial := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	var connectWG sync.WaitGroup
	connectWG.Add(1)
	connectErr := make(chan error)
	go func() {
		defer connectWG.Done()
		connectErr <- client.connectWith(ctx, func(_ string, _ ...nats.Option) (*nats.Conn, error) {
			close(dialEntered)
			<-releaseDial
			return candidate, nil
		})
	}()

	<-dialEntered
	cancel()
	close(releaseDial)
	err = <-connectErr
	connectWG.Wait()

	require.ErrorIs(t, err, context.Canceled)
	require.True(t, candidate.IsClosed(), "canceled Connect must close its rejected native candidate")
	require.Nil(t, client.GetConnection())
	require.Nil(t, client.js)
	require.Equal(t, StatusDisconnected, client.Status())
}

func TestConnectRejectsCandidateWhenCloseWinsAdmission(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)
	client.healthInterval = 0

	candidate := newConnectCandidate(t)
	dialEntered := make(chan struct{})
	releaseDial := make(chan struct{})

	var connectWG sync.WaitGroup
	connectWG.Add(1)
	connectErr := make(chan error)
	go func() {
		defer connectWG.Done()
		connectErr <- client.connectWith(context.Background(), func(_ string, _ ...nats.Option) (*nats.Conn, error) {
			close(dialEntered)
			<-releaseDial
			return candidate, nil
		})
	}()

	<-dialEntered
	closeCtx, cancelClose := context.WithTimeout(context.Background(), closeBudget)
	defer cancelClose()
	require.NoError(t, client.Close(closeCtx))
	close(releaseDial)
	err = <-connectErr
	connectWG.Wait()

	require.ErrorIs(t, err, nats.ErrConnectionClosed)
	require.True(t, candidate.IsClosed(), "Connect must close a candidate rejected by terminal Close")
	require.Nil(t, client.GetConnection())
	require.Nil(t, client.js)
	require.Equal(t, StatusDisconnected, client.Status())
}

func TestConnectCommitsCandidateWhenAdmissionRemainsOpen(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)
	client.healthInterval = 0

	candidate := newConnectCandidate(t)
	err = client.connectWith(context.Background(), func(_ string, _ ...nats.Option) (*nats.Conn, error) {
		return candidate, nil
	})
	require.NoError(t, err)

	require.Same(t, candidate, client.GetConnection())
	require.NotNil(t, client.js)
	require.False(t, candidate.IsClosed())
	require.Equal(t, StatusConnected, client.Status())

	closeCtx, cancelClose := context.WithTimeout(context.Background(), closeBudget)
	defer cancelClose()
	require.NoError(t, client.Close(closeCtx))
}

func newConnectCandidate(t *testing.T) *nats.Conn {
	t.Helper()

	server, err := natsserver.NewServer(&natsserver.Options{
		Port:   -1,
		NoLog:  true,
		NoSigs: true,
	})
	require.NoError(t, err)
	server.Start()
	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
	})

	candidate, err := nats.Connect("", nats.InProcessServer(server))
	require.NoError(t, err)
	t.Cleanup(candidate.Close)
	return candidate
}

// connectWait bounds how long a test waits for Connect against a server that
// does not answer. Before design D2, Connect waited for nats.go's connect
// timeout instead (5 s per address, P18), so a Connect that honours its
// context returns well inside it.
const connectWait = 3 * time.Second

// T3, design "Tests that replace the probes".
func TestConnectRefusesAnEndedContext(t *testing.T) {
	url, accepted := silentListener(t)
	client, err := NewClient(url)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = client.Connect(ctx)

	require.ErrorIs(t, err, context.Canceled)
	require.True(t, errs.IsTransient(err), "an ended context is transient: %v", err)
	select {
	case <-accepted:
		t.Error("Connect dialled with an ended context: the listener accepted a connection")
	default:
	}
}

// T4, design "Tests that replace the probes".
func TestConnectEndsWithItsContext(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		url, _ := silentListener(t)
		client, err := NewClient(url)
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()

		err = awaitConnect(t, startConnect(ctx, client))
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("cancel once accepted", func(t *testing.T) {
		url, accepted := silentListener(t)
		client, err := NewClient(url)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		result := startConnect(ctx, client)
		select {
		case <-accepted:
		case err := <-result:
			t.Fatalf("Connect returned before the listener accepted: %v", err)
		}
		cancel()
		require.ErrorIs(t, awaitConnect(t, result), context.Canceled)
	})

	t.Run("two addresses", func(t *testing.T) {
		first, _ := silentListener(t)
		second, _ := silentListener(t)
		client, err := NewClient(first + "," + second)
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()

		err = awaitConnect(t, startConnect(ctx, client))
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

// startConnect runs Connect in its own goroutine; awaitConnect receives its
// result.
func startConnect(ctx context.Context, client *Client) <-chan error {
	result := make(chan error, 1)
	go func() { result <- client.Connect(ctx) }()
	return result
}

// awaitConnect returns the result of a Connect started by startConnect, and
// fails the test when it is not there within connectWait. It then still waits
// for it, so no Connect outlives the test; nats.go's connect timeout bounds
// that wait.
func awaitConnect(t *testing.T, result <-chan error) error {
	t.Helper()
	start := time.Now()
	wait := time.NewTimer(connectWait)
	defer wait.Stop()
	select {
	case err := <-result:
		return err
	case <-wait.C:
		err := <-result
		t.Errorf("Connect did not return within %v; it returned after %v with %v",
			connectWait, time.Since(start).Round(time.Millisecond), err)
		return err
	}
}

// silentListener listens on an ephemeral loopback port and accepts connections
// without ever answering, as a NATS server that does not respond. A value is
// ready on accepted once it has accepted a connection. Cleanup closes the
// listener and every connection it accepted.
func silentListener(t *testing.T) (url string, accepted <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	signal := make(chan struct{}, 1)
	done := make(chan struct{})
	var conns []net.Conn // written by the accept loop; read after done is closed
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns = append(conns, c)
			select {
			case signal <- struct{}{}:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return "nats://" + ln.Addr().String(), signal
}
