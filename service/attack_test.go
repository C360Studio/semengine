package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The status route under hostile input, from the pin's attack_test.go. The pin built struct-literal
// managers holding a hand-filled component map; here every manager is built through
// NewComponentManager (design D7).

// TestAttack_HandleComponentStatus_MaliciousQueryParams: hostile debug values do not panic.
func TestAttack_HandleComponentStatus_MaliciousQueryParams(t *testing.T) {
	cm := newAttackComponentManager(t)

	maliciousParams := []struct {
		name string
		url  string
	}{
		{"xss attempt", "/components/status/test-component?debug=%3Cscript%3Ealert%28%27xss%27%29%3C%2Fscript%3E"},
		{"path traversal", "/components/status/test-component?debug=../../etc/passwd"},
		{"extremely long", "/components/status/test-component?debug=" + strings.Repeat("x", 10000)},
		{"unicode exploit", "/components/status/test-component?debug=%E2%80%AE%E2%80%AD"},
		{"multiple params", "/components/status/test-component?debug=true&debug=false&debug=malicious"},
		{"empty param", "/components/status/test-component?debug="},
		{"no value", "/components/status/test-component?debug"},
	}

	for _, tc := range maliciousParams {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			w := httptest.NewRecorder()

			require.NotPanics(t, func() {
				cm.handleComponentStatus("/components/status/")(w, req)
			})
		})
	}
}

// TestAttack_HandleComponentStatus_ConcurrentRequests: concurrent status requests each answer 200
// or 404 and none panics.
func TestAttack_HandleComponentStatus_ConcurrentRequests(t *testing.T) {
	cm := newAttackComponentManager(t)

	const numRequests = 100
	var wg sync.WaitGroup
	wg.Add(numRequests)

	failures := make(chan error, numRequests)

	for i := range numRequests {
		go func(debugParam string) {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodGet, "/components/status/test-component?debug="+debugParam, nil)
			w := httptest.NewRecorder()

			defer func() {
				if r := recover(); r != nil {
					failures <- assert.AnError
				}
			}()

			cm.handleComponentStatus("/components/status/")(w, req)

			if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
				failures <- assert.AnError
			}
		}([]string{"true", "false", "invalid"}[i%3])
	}

	wg.Wait()
	close(failures)

	failureCount := 0
	for range failures {
		failureCount++
	}

	assert.Equal(t, 0, failureCount, "Should handle concurrent requests without errors")
}

// TestAttack_HandleComponentStatus_NoGoroutineLeak: status requests leave no goroutine running.
// The pin slept 100ms and compared runtime.NumGoroutine with a tolerance of five; here the
// requests run inside a synctest bubble, which returns only once every goroutine started in it
// has exited, so a goroutine a request leaves behind fails the test (docs/testing.md, "Services
// and the lifecycle suite"). The manager is built outside the bubble; nothing in it reaches the
// NATS client.
func TestAttack_HandleComponentStatus_NoGoroutineLeak(t *testing.T) {
	cm := newAttackComponentManager(t)

	synctest.Test(t, func(t *testing.T) {
		for range 100 {
			req := httptest.NewRequest(http.MethodGet, "/components/status/test-component?debug=true", nil)
			w := httptest.NewRecorder()
			cm.handleComponentStatus("/components/status/")(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		}
	})
}

// TestAttack_HandleComponentStatus_MaliciousComponentName: a name that is "." or "..", or that
// decodes once to one holding a path separator, is a 400 (design D5); a double-encoded or very long
// name decodes to a valid name no component has, so it is a 404. None panics.
func TestAttack_HandleComponentStatus_MaliciousComponentName(t *testing.T) {
	cm := newAttackComponentManager(t)

	maliciousNames := []struct {
		name string
		path string
		want int
	}{
		{"path traversal", "/components/status/../../etc/passwd?debug=true", http.StatusBadRequest},
		{"url encoded slash", "/components/status/test%2Fcomponent?debug=true", http.StatusBadRequest},
		{"double encoded", "/components/status/test%252Fcomponent?debug=true", http.StatusNotFound},
		{"extremely long", "/components/status/" + strings.Repeat("a", 10000) + "?debug=true", http.StatusNotFound},
		{"dot dot", "/components/status/..?debug=true", http.StatusBadRequest},
		{"single dot", "/components/status/.?debug=true", http.StatusBadRequest},
	}

	for _, tc := range maliciousNames {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()

			require.NotPanics(t, func() {
				cm.handleComponentStatus("/components/status/")(w, req)
			})

			assert.Equal(t, tc.want, w.Code, "GET %s", tc.path)
		})
	}
}

// TestAttack_HandleComponentStatus_JSONBomb: a deeply nested debug payload is served as valid JSON.
func TestAttack_HandleComponentStatus_JSONBomb(t *testing.T) {
	cm := newTestComponentManager(t, testComponent{
		name: "bomb-component", instance: &attackMockComponentMassiveDebug{},
	})

	req := httptest.NewRequest(http.MethodGet, "/components/status/bomb-component?debug=true", nil)
	w := httptest.NewRecorder()

	require.NotPanics(t, func() {
		cm.handleComponentStatus("/components/status/")(w, req)
	})

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response), "Should return valid JSON even with large payload")
}

// TestAttack_HandleComponentStatus_RapidRequests: a thousand requests in a row do not panic.
func TestAttack_HandleComponentStatus_RapidRequests(t *testing.T) {
	cm := newAttackComponentManager(t)

	for range 1000 {
		req := httptest.NewRequest(http.MethodGet, "/components/status/test-component?debug=true", nil)
		w := httptest.NewRecorder()

		require.NotPanics(t, func() {
			cm.handleComponentStatus("/components/status/")(w, req)
		})
	}
}

// TestAttack_HandleComponentStatus_MemoryExhaustion: an extremely long query does not panic.
func TestAttack_HandleComponentStatus_MemoryExhaustion(t *testing.T) {
	cm := newAttackComponentManager(t)

	longQuery := strings.Repeat("debug=true&", 10000)
	req := httptest.NewRequest(http.MethodGet, "/components/status/test-component?"+longQuery, nil)
	w := httptest.NewRecorder()

	require.NotPanics(t, func() {
		cm.handleComponentStatus("/components/status/")(w, req)
	})
}

// TestAttack_HandleComponentStatus_InvalidHTTPMethods: every method but GET is a 405.
func TestAttack_HandleComponentStatus_InvalidHTTPMethods(t *testing.T) {
	cm := newAttackComponentManager(t)

	methods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodHead,
		http.MethodOptions,
		http.MethodConnect,
		http.MethodTrace,
		"INVALID",
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/components/status/test-component?debug=true", nil)
			w := httptest.NewRecorder()

			require.NotPanics(t, func() {
				cm.handleComponentStatus("/components/status/")(w, req)
			})

			assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		})
	}
}

// attackMockComponentMassiveDebug reports a debug status nested a hundred levels deep.
type attackMockComponentMassiveDebug struct {
	baseDiscoverable
}

func (m *attackMockComponentMassiveDebug) DebugStatus() any {
	bomb := make(map[string]any)
	current := bomb
	for range 100 {
		next := make(map[string]any)
		current["nested"] = next
		current = next
	}
	return bomb
}

// newAttackComponentManager builds a manager composing test-component, a
// component.DebugStatusProvider.
func newAttackComponentManager(t *testing.T) *ComponentManager {
	t.Helper()
	return newTestComponentManager(t, testComponent{
		name: "test-component", instance: &attackMockComponentWithDebug{},
	})
}

// attackMockComponentWithDebug reports a small debug status that includes a timestamp.
type attackMockComponentWithDebug struct {
	baseDiscoverable
}

func (m *attackMockComponentWithDebug) DebugStatus() any {
	return map[string]any{
		"test_field": "test_value",
		"timestamp":  time.Now(),
	}
}
