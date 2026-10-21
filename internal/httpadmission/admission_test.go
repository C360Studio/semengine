package httpadmission

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRequestsRefuseAfterClose (moved from metric/admission_test.go with the counter,
// setup-04a-03-boot-service-manager design D2): a request arriving after admission
// closed gets 503 with the caller's refusal text and its handler never runs;
// admission closed with nothing running is observed as complete.
func TestRequestsRefuseAfterClose(t *testing.T) {
	var requests Requests
	ran := false
	handler := requests.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { ran = true }), "widget server is stopping")
	requests.CloseAdmission()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	require.False(t, ran, "a request after admission closed must not run the handler")
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "widget server is stopping")
	require.Equal(t, "close", recorder.Header().Get("Connection"))
	require.Zero(t, requests.Pending())
	require.NoError(t, requests.Wait(t.Context()))
}

// TestRequestsWaitForAnAdmittedRequest: Wait returns its context's error while an
// admitted request runs, even after admission closed, and nil once it returns.
func TestRequestsWaitForAnAdmittedRequest(t *testing.T) {
	var requests Requests
	entered, release := make(chan struct{}), make(chan struct{})
	handler := requests.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}), "stopping")
	served := make(chan struct{})
	go func() {
		defer close(served)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}()
	<-entered
	requests.CloseAdmission()
	require.Equal(t, 1, requests.Pending())

	ended, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, requests.Wait(ended), context.Canceled)

	close(release)
	<-served
	require.NoError(t, requests.Wait(t.Context()))
	require.Zero(t, requests.Pending())
}
