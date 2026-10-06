//go:build integration

package natsclient

import (
	"context"
	"testing"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

func TestInternalConsumerReturnsNativeHandleAndRejectsDuplicateIncumbent(t *testing.T) {
	testClient := newFixtureClient(t)
	testClient.stream(t, "I1_INTERNAL", "i1.internal.>")
	cfg := StreamConsumerConfig{
		StreamName: "I1_INTERNAL", ConsumerName: "i1-fixed",
		FilterSubject: "i1.internal.>", AckPolicy: "explicit", DeliverPolicy: "all",
	}
	delivered := make(chan struct{}, 1)
	handle, err := testClient.Client.ConsumeInternalStreamWithConfig(
		t.Context(), cfg, func(_ context.Context, msg jetstream.Msg) {
			delivered <- struct{}{}
			_ = msg.Ack()
		},
	)
	require.NoError(t, err)
	require.NotNil(t, handle)

	duplicate, duplicateErr := testClient.Client.ConsumeInternalStreamWithConfig(
		t.Context(), cfg, func(context.Context, jetstream.Msg) {},
	)
	require.Nil(t, duplicate)
	require.Error(t, duplicateErr)
	require.True(t, errs.IsInvalid(duplicateErr))

	require.NoError(t, testClient.Client.PublishToStream(t.Context(), "i1.internal.work", []byte("work")))
	<-delivered

	handle.Drain()
	<-handle.Closed()
	waitForInternalClaimRelease(t, testClient.Client, internalConsumerIdentity{
		stream: cfg.StreamName, durable: cfg.ConsumerName,
	})
	reacquired, err := testClient.Client.ConsumeInternalStreamWithConfig(
		t.Context(), cfg, func(context.Context, jetstream.Msg) {},
	)
	require.NoError(t, err)
	reacquired.Drain()
	<-reacquired.Closed()
	waitForInternalClaimRelease(t, testClient.Client, internalConsumerIdentity{
		stream: cfg.StreamName, durable: cfg.ConsumerName,
	})
}

func TestInternalConsumerSetupFailureReleasesReservationWithoutDelivery(t *testing.T) {
	testClient := newFixtureClient(t)
	testClient.stream(t, "I1_SETUP", "i1.setup.>")
	cfg := StreamConsumerConfig{
		StreamName: "I1_SETUP", ConsumerName: "i1-setup-fixed",
		FilterSubject: "i1.setup.>.invalid", AckPolicy: "explicit", DeliverPolicy: "all",
	}
	delivered := make(chan struct{}, 1)
	handle, err := testClient.Client.ConsumeInternalStreamWithConfig(
		t.Context(), cfg, func(context.Context, jetstream.Msg) { delivered <- struct{}{} },
	)
	require.Nil(t, handle)
	require.Error(t, err)
	select {
	case <-delivered:
		t.Fatal("setup failure began delivery")
	default:
	}

	cfg.FilterSubject = "i1.setup.>"
	handle, err = testClient.Client.ConsumeInternalStreamWithConfig(
		t.Context(), cfg, func(context.Context, jetstream.Msg) {},
	)
	require.NoError(t, err, "failed setup must release the exact local reservation")
	handle.Drain()
	<-handle.Closed()
	waitForInternalClaimRelease(t, testClient.Client, internalConsumerIdentity{
		stream: cfg.StreamName, durable: cfg.ConsumerName,
	})
}

func TestPortConsumerReturnsNativeOwnershipWithoutClientCatalog(t *testing.T) {
	testClient := newFixtureClient(t)
	testClient.stream(t, "S1_PORT", "s1.port.>")
	cfg := StreamConsumerConfig{
		StreamName: "S1_PORT", ConsumerName: "s1-fixed",
		FilterSubject: "s1.port.>", AckPolicy: "explicit", DeliverPolicy: "all",
	}
	owner := PortConsumerContext{Component: "s1-owner", Port: "input"}
	delivered := make(chan struct{}, 1)
	handle, err := testClient.Client.ConsumeStreamWithConfig(
		t.Context(), owner, cfg, func(_ context.Context, msg jetstream.Msg) {
			delivered <- struct{}{}
			_ = msg.Ack()
		},
	)
	require.NoError(t, err)
	require.NotNil(t, handle)

	duplicate, duplicateErr := testClient.Client.ConsumeStreamWithConfig(
		t.Context(), owner, cfg, func(context.Context, jetstream.Msg) {},
	)
	require.Nil(t, duplicate)
	require.Error(t, duplicateErr)
	require.True(t, errs.IsInvalid(duplicateErr))

	require.NoError(t, testClient.Client.PublishToStream(t.Context(), "s1.port.work", []byte("work")))
	<-delivered
	handle.Drain()
	<-handle.Closed()
	waitForInternalClaimRelease(t, testClient.Client, internalConsumerIdentity{
		stream: cfg.StreamName, durable: cfg.ConsumerName,
	})

	reacquired, err := testClient.Client.ConsumeStreamWithConfig(
		t.Context(), owner, cfg, func(context.Context, jetstream.Msg) {},
	)
	require.NoError(t, err)
	reacquired.Drain()
	<-reacquired.Closed()
	waitForInternalClaimRelease(t, testClient.Client, internalConsumerIdentity{
		stream: cfg.StreamName, durable: cfg.ConsumerName,
	})
}

func TestPortConsumerSetupFailureReleasesReservationWithoutDelivery(t *testing.T) {
	testClient := newFixtureClient(t)
	testClient.stream(t, "S1_PORT_SETUP", "s1.port.setup.>")
	cfg := StreamConsumerConfig{
		StreamName: "S1_PORT_SETUP", ConsumerName: "s1-setup-fixed",
		FilterSubject: "s1.port.setup.>.invalid", AckPolicy: "explicit", DeliverPolicy: "all",
	}
	owner := PortConsumerContext{Component: "s1-owner", Port: "input"}
	delivered := make(chan struct{}, 1)
	handle, err := testClient.Client.ConsumeStreamWithConfig(
		t.Context(), owner, cfg, func(context.Context, jetstream.Msg) { delivered <- struct{}{} },
	)
	require.Nil(t, handle)
	require.Error(t, err)
	select {
	case <-delivered:
		t.Fatal("setup failure began delivery")
	default:
	}

	cfg.FilterSubject = "s1.port.setup.>"
	handle, err = testClient.Client.ConsumeStreamWithConfig(
		t.Context(), owner, cfg, func(context.Context, jetstream.Msg) {},
	)
	require.NoError(t, err, "failed setup must release the exact local reservation")
	handle.Drain()
	<-handle.Closed()
	waitForInternalClaimRelease(t, testClient.Client, internalConsumerIdentity{
		stream: cfg.StreamName, durable: cfg.ConsumerName,
	})
}

func waitForInternalClaimRelease(t *testing.T, client *Client, identity internalConsumerIdentity) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	_, err := probe.Await(ctx, func(context.Context) (bool, error) {
		client.internalClaimsMu.Lock()
		defer client.internalClaimsMu.Unlock()
		_, active := client.internalClaims[identity]
		return active, nil
	}, func(active bool) bool { return !active })
	require.NoError(t, err, "internal claim %+v not released", identity)
}

func waitForConsumerObservationRemoval(t *testing.T, metrics *jetstreamMetrics, key string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	_, err := probe.Await(ctx, func(context.Context) (bool, error) {
		metrics.mu.Lock()
		defer metrics.mu.Unlock()
		_, active := metrics.consumers[key]
		return active, nil
	}, func(active bool) bool { return !active })
	require.NoError(t, err, "consumer observation %q not removed", key)
}
