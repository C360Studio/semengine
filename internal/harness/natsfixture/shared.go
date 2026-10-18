package natsfixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	// startBudget bounds the shared server's first-reset start, matching the removed runner's
	// 300-second image-pull budget so a cold pull on CI fits (nats-fixture › "Shared server per
	// package").
	startBudget = 5 * time.Minute
	// resetBudget bounds the reset's stream deletion and AccountInfo verification against a server
	// that is already running.
	resetBudget = 15 * time.Second
	// connWaitBudget bounds the STATZ poll for earlier or leaked client connections to finish
	// closing across Docker's port forwarding.
	connWaitBudget = 2 * time.Second
	// statzSubject is the server's account-statistics subject, imported into every account
	// (nats-server v2.15.0 server/events.go:2404; probe on PR #206 comment 6133623880).
	statzSubject = "$SYS.REQ.ACCOUNT.PING.STATZ"
	// defaultAccount is the global default account nats-server places clients in when started
	// without an accounts configuration.
	defaultAccount = "$G"
)

// Shared is one package's NATS server, started at the first Reset and removed by TestMain's Stop
// (nats-fixture › "Shared server per package", "Reset to a verified-empty server"). It retains no
// context.Context. The zero value is ready to use.
type Shared struct {
	mu       sync.Mutex
	f        Fixture
	started  bool
	startErr error
	stopped  bool
}

// Reset starts the shared server on its first call, empties it, verifies it holds no stream,
// consumer, stored byte or other client connection, registers a post-test connection check on t's
// cleanup, and returns the server's client URL.
func (s *Shared) Reset(t testing.TB) string {
	t.Helper()
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		t.Fatalf("natsfixture: Reset on a stopped shared server")
		return ""
	}
	if !s.started {
		s.started = true
		s.f.errorf = t.Errorf
		s.f.logf = t.Logf
		if s.f.readPort == nil {
			s.f.readPort = readMappedPort
		}
		startCtx, cancel := context.WithTimeout(t.Context(), startBudget)
		err := s.f.Start(startCtx)
		cancel()
		if err != nil {
			s.startErr = err
			s.mu.Unlock()
			t.Fatalf("natsfixture: start shared server: %v", err)
			return ""
		}
		// Close the connection Start made before the reset counts client connections.
		s.f.mu.Lock()
		nc := s.f.nc
		s.f.nc, s.f.js = nil, nil
		s.f.mu.Unlock()
		if nc != nil {
			nc.Close()
		}
	} else if s.startErr != nil {
		err := s.startErr
		s.mu.Unlock()
		t.Fatalf("natsfixture: start shared server: %v", err)
		return ""
	}
	url := s.f.URL()
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(t.Context(), resetBudget)
	defer cancel()
	nc, err := connect(ctx, url)
	if err != nil {
		t.Fatalf("natsfixture: reset connect %s: %v", url, err)
		return ""
	}
	defer nc.Close()

	waitCtx, waitCancel := context.WithTimeout(ctx, connWaitBudget)
	other, err := awaitOnlySelf(waitCtx, nc)
	waitCancel()
	if err != nil {
		t.Fatalf("natsfixture: reset: a connection from an earlier test is still open (%d other client connection(s); %v)", other, err)
		return ""
	}

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("natsfixture: reset jetstream: %v", err)
		return ""
	}
	names := js.StreamNames(ctx)
	var streams []string
	for name := range names.Name() {
		streams = append(streams, name)
	}
	if err := names.Err(); err != nil {
		t.Fatalf("natsfixture: reset list streams: %v", err)
		return ""
	}
	for _, name := range streams {
		if err := js.DeleteStream(ctx, name); err != nil {
			t.Fatalf("natsfixture: reset delete stream %s: %v", name, err)
			return ""
		}
	}
	info, err := js.AccountInfo(ctx)
	if err != nil {
		t.Fatalf("natsfixture: reset account info: %v", err)
		return ""
	}
	if info.Streams != 0 || info.Consumers != 0 || info.Memory != 0 || info.Store != 0 {
		t.Fatalf("natsfixture: server not empty after reset: streams=%d consumers=%d memory=%d store=%d",
			info.Streams, info.Consumers, info.Memory, info.Store)
		return ""
	}
	nc.Close()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), connWaitBudget)
		defer cancel()
		checkConn, err := connect(ctx, url)
		if err != nil {
			t.Errorf("natsfixture: connection check after test: connect %s: %v", url, err)
			return
		}
		defer checkConn.Close()
		if other, err := awaitOnlySelf(ctx, checkConn); err != nil {
			t.Errorf("natsfixture: %d client connection(s) still open after the test ended (%v)", other, err)
		}
	})
	return url
}

// Stop removes the shared server's container when it started, and returns nil without a Docker
// call when it did not.
func (s *Shared) Stop(ctx context.Context) error {
	if ctx == nil {
		return errors.New("natsfixture: Stop with a nil context")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.f.Stop(ctx)
	if err == nil && s.started && s.startErr == nil {
		s.stopped = true
	}
	return err
}

// awaitOnlySelf polls STATZ until the default account reports only nc itself (conns == 1),
// returning the last observed count of other client connections when the wait fails.
func awaitOnlySelf(ctx context.Context, nc *nats.Conn) (int, error) {
	lastConns := 1
	conns, err := probe.Await(ctx, func(ctx context.Context) (int, error) {
		c, err := statzConns(ctx, nc)
		if err == nil {
			lastConns = c
		}
		return c, err
	}, func(conns int) bool {
		return conns == 1
	})
	if conns == 0 {
		conns = lastConns
	}
	return max(conns-1, 0), err
}

type statzReply struct {
	Data struct {
		Accounts []struct {
			Account string `json:"acc"`
			Conns   int    `json:"conns"`
		} `json:"account_statz"`
	} `json:"data"`
}

func statzConns(ctx context.Context, nc *nats.Conn) (int, error) {
	msg, err := nc.RequestWithContext(ctx, statzSubject, nil)
	if err != nil {
		return 0, err
	}
	var reply statzReply
	if err := json.Unmarshal(msg.Data, &reply); err != nil {
		return 0, err
	}
	for _, a := range reply.Data.Accounts {
		if a.Account == defaultAccount {
			return a.Conns, nil
		}
	}
	return 0, fmt.Errorf("account %s absent from %s reply", defaultAccount, statzSubject)
}
