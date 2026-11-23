package httpembedder_test

import (
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semengine/graph/embedding/httpembedder"
	"github.com/c360studio/semengine/pkg/errs"
)

// TestProviderUnavailableIsMarked holds the embedder to the line between a provider that is busy
// or failing, which may answer later, and one that refused the request, which will refuse it
// again. The first kind is marked with pkg/errs so a caller can retry it without reading text.
func TestProviderUnavailableIsMarked(t *testing.T) {
	t.Run("429_is_rate_limited_after_one_request", func(t *testing.T) {
		var requests atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			// An hour: an embedder that honoured it would hold the test until go test's timeout.
			w.Header().Set("Retry-After", "3600")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down","type":"rate_limit_error"}}`))
		}))
		t.Cleanup(srv.Close)
		e := newEmbedder(t, httpembedder.Config{BaseURL: srv.URL + "/v1", Model: testModel})

		got, err := e.Generate(t.Context(), []string{"a"})
		if err == nil || got != nil {
			t.Fatalf("Generate = %v, %v; want an error and no vectors", got, err)
		}
		if !errors.Is(err, errs.ErrRateLimited) {
			t.Fatalf("error %q does not match errs.ErrRateLimited", err)
		}
		if n := requests.Load(); n != 1 {
			t.Fatalf("provider saw %d requests, want 1", n)
		}
	})

	for _, status := range []int{
		http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout,
	} {
		t.Run(http.StatusText(status)+"_is_transient", func(t *testing.T) {
			p := newProvider(t, fixed(status, errorReply{errorDetail{"model is loading", "server_error"}}))
			e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: testModel})
			if _, err := e.Generate(t.Context(), []string{"a"}); !errs.IsTransient(err) {
				t.Fatalf("status %d: error %v is not transient", status, err)
			}
		})
	}

	t.Run("closed_server_is_transient", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		base := srv.URL + "/v1"
		srv.Close()
		e := newEmbedder(t, httpembedder.Config{BaseURL: base, Model: testModel})
		if _, err := e.Generate(t.Context(), []string{"a"}); !errs.IsTransient(err) {
			t.Fatalf("error %v from a closed server is not transient", err)
		}
	})

	for _, status := range []int{
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound,
		http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity, http.StatusNotImplemented,
	} {
		t.Run(http.StatusText(status)+"_is_not_transient", func(t *testing.T) {
			p := newProvider(t, fixed(status, errorReply{errorDetail{"refused", "invalid_request_error"}}))
			e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: testModel})
			_, err := e.Generate(t.Context(), []string{"a"})
			if err == nil {
				t.Fatalf("status %d: Generate returned no error", status)
			}
			if errs.IsTransient(err) {
				t.Fatalf("status %d: error %v is transient", status, err)
			}
		})
	}

	t.Run("untrusted_reply_is_not_transient", func(t *testing.T) {
		p := newProvider(t, fixed(http.StatusOK, "not json"))
		e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: testModel})
		_, err := e.Generate(t.Context(), []string{"a"})
		if err == nil {
			t.Fatal("Generate accepted a reply that is not JSON")
		}
		if errs.IsTransient(err) {
			t.Fatalf("error %v for an untrusted reply is transient", err)
		}
	})

	// This drives the TLS client of the Go release go.mod names against a certificate it does
	// not trust; the error it returns is what the embedder must leave unmarked.
	t.Run("untrusted_certificate_is_not_transient", func(t *testing.T) {
		srv := httptest.NewUnstartedServer(http.NotFoundHandler())
		// The server logs the handshake the client abandons; that line is expected, not a fault.
		srv.Config.ErrorLog = log.New(io.Discard, "", 0)
		srv.StartTLS()
		t.Cleanup(srv.Close)
		e := newEmbedder(t, httpembedder.Config{BaseURL: srv.URL + "/v1", Model: testModel})
		_, err := e.Generate(t.Context(), []string{"a"})
		if err == nil {
			t.Fatal("Generate trusted a certificate nothing vouches for")
		}
		var certErr *tls.CertificateVerificationError
		if !errors.As(err, &certErr) {
			t.Fatalf("error %v is not a *tls.CertificateVerificationError", err)
		}
		if errs.IsTransient(err) {
			t.Fatalf("error %v for an untrusted certificate is transient", err)
		}
	})
}
