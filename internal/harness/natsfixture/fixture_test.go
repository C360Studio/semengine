package natsfixture

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// startedWith returns a fixture that looks started to its resource methods, with dependency
// calls captured instead of sent: enough to prove what the fixture asks the broker for.
func startedWith(t *testing.T) *Fixture {
	t.Helper()
	f := &Fixture{testName: t.Name(), deps: defaultDeps(), calls: map[string]int{}, containerID: "c0ffee"}
	f.js = struct{ jetstream.JetStream }{} // never called: every JetStream use below goes through deps
	return f
}

// Every stream and bucket the fixture creates declares its bounds (nats-fixture › "Run-unique
// ownership and safe names"); the values are restated here, not read from the implementation.
func TestCreateStreamDeclaresBounds(t *testing.T) {
	f := startedWith(t)
	var got jetstream.StreamConfig
	f.deps.createStream = func(_ context.Context, _ jetstream.JetStream, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
		got = cfg
		return nil, nil
	}
	if _, err := f.CreateStream(t.Context(), "orders", "orders.>"); err != nil {
		t.Fatal(err)
	}
	if got.Name != "orders" || len(got.Subjects) != 1 || got.MaxAge != time.Hour || got.MaxBytes != 64<<20 || got.Discard != jetstream.DiscardOld {
		t.Fatalf("stream config %+v lacks the declared bounds", got)
	}
	if rem := f.remaining(); len(rem) != 1 || rem[0] != "stream orders" {
		t.Fatalf("created stream not owned: %v", rem)
	}
}

func TestCreateKeyValueDeclaresBounds(t *testing.T) {
	f := startedWith(t)
	var got jetstream.KeyValueConfig
	f.deps.createKV = func(_ context.Context, _ jetstream.JetStream, cfg jetstream.KeyValueConfig) (jetstream.KeyValue, error) {
		got = cfg
		return nil, nil
	}
	if _, err := f.CreateKeyValue(t.Context(), "state"); err != nil {
		t.Fatal(err)
	}
	if got.Bucket != "state" || got.TTL != time.Hour || got.MaxBytes != 64<<20 {
		t.Fatalf("bucket config %+v lacks the declared bounds", got)
	}
	if rem := f.remaining(); len(rem) != 1 || rem[0] != "bucket state" {
		t.Fatalf("created bucket not owned: %v", rem)
	}
}

// A create that fails is still owned: it may have happened on the broker, and Stop treats a
// resource that turns out not to exist as absent.
func TestFailedCreateIsOwnedAndTyped(t *testing.T) {
	f := startedWith(t)
	cause := errors.New("timeout")
	f.deps.createStream = func(context.Context, jetstream.JetStream, jetstream.StreamConfig) (jetstream.Stream, error) {
		return nil, cause
	}
	_, err := f.CreateStream(t.Context(), "s")
	var fe *Error
	if !errors.As(err, &fe) || fe.Phase != PhaseCreateStream || !errors.Is(err, cause) || fe.ContainerID != "c0ffee" {
		t.Fatalf("CreateStream = %v, want a typed create-stream error", err)
	}
	if rem := f.remaining(); len(rem) != 1 {
		t.Fatalf("failed create not owned: %v", rem)
	}
}

func TestResourceMethodsRefuseBeforeStartAndNilContexts(t *testing.T) {
	f := New(t)
	var nilCtx context.Context
	checks := map[string]error{}
	_, checks["CreateStream before Start"] = f.CreateStream(t.Context(), "s")
	_, checks["CreateKeyValue before Start"] = f.CreateKeyValue(t.Context(), "b")
	_, checks["Consume before Start"] = f.Consume(t.Context(), "s", "c", func(context.Context, jetstream.Msg) {})
	_, checks["CreateStream nil ctx"] = f.CreateStream(nilCtx, "s")
	_, checks["CreateKeyValue nil ctx"] = f.CreateKeyValue(nilCtx, "b")
	_, checks["Consume nil handler"] = f.Consume(t.Context(), "s", "c", nil)
	checks["Start nil ctx"] = f.Start(nilCtx)
	checks["Stop nil ctx"] = f.Stop(nilCtx)
	for name, err := range checks {
		if err == nil {
			t.Errorf("%s returned nil", name)
		}
	}
	if f.totalCalls() != 0 {
		t.Errorf("refusals made calls: %v", f.callCounts())
	}
}

func TestPreCancelledStartIsRefusedWithoutConsuming(t *testing.T) {
	f := New(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := f.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start = %v, want context.Canceled", err)
	}
	if f.used || f.totalCalls() != 0 {
		t.Fatal("a pre-cancelled Start consumed the fixture or made a call")
	}
}

func TestErrorReportsEveryField(t *testing.T) {
	cause, cleanup := errors.New("dial refused"), errors.New("terminate: daemon gone")
	e := &Error{Attempt: 2, Phase: PhaseConnect, ContainerID: "0123456789abcdef", ParentErr: context.Canceled, Cause: cause, Cleanup: cleanup}
	msg := e.Error()
	for _, want := range []string{"attempt 2", "phase connect", "container 0123456789ab", "parent context context canceled", "dial refused", "cleanup: terminate: daemon gone"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q lacks %q", msg, want)
		}
	}
	if !errors.Is(e, cause) || !errors.Is(e, cleanup) {
		t.Error("Unwrap hides the cause or the cleanup error")
	}
	live := (&Error{Attempt: 1, Phase: PhaseStart, Cause: cause}).Error()
	if !strings.Contains(live, "parent context live") || strings.Contains(live, "container") {
		t.Errorf("%q", live)
	}
}

func TestRollbackIgnoresParentCancellationButStaysBounded(t *testing.T) {
	var nilCtx context.Context
	if err := rollback(nilCtx, func(context.Context) error { return nil }); err == nil {
		t.Fatal("rollback accepted a nil parent")
	}
	parent, cancel := context.WithCancel(t.Context())
	cancel()
	var sawErr error
	var sawDeadline bool
	if err := rollback(parent, func(ctx context.Context) error {
		sawErr = ctx.Err()
		_, sawDeadline = ctx.Deadline()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if sawErr != nil || !sawDeadline {
		t.Fatalf("rollback context: err %v deadline %t; want live and bounded after parent cancellation", sawErr, sawDeadline)
	}
}
