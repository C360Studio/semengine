package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/types"
)

// The component manager's routes, from the pin's component_manager_http_test.go. The pin built
// struct-literal managers and filled their component map by hand; here every manager is built
// through NewComponentManager over a test registry and configuration (design D7), and the
// flowgraph answer is compared with checked-in files (design D5).

// TestHandleComponentStatus_WithoutDebugParam: without a debug parameter the status carries the
// component's name and state and no debug field.
func TestHandleComponentStatus_WithoutDebugParam(t *testing.T) {
	cm := newDebugStatusComponentManager(t)

	req := httptest.NewRequest(http.MethodGet, "/components/status/test-component", nil)
	w := httptest.NewRecorder()

	cm.handleComponentStatus("/components/status/")(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, "test-component", response["name"])
	assert.NotEmpty(t, response["state"])

	_, hasDebug := response["debug"]
	assert.False(t, hasDebug, "Response should not have debug field without debug parameter")
}

// TestHandleComponentStatus_DebugParamFalse: debug=false adds no debug field.
func TestHandleComponentStatus_DebugParamFalse(t *testing.T) {
	cm := newDebugStatusComponentManager(t)

	req := httptest.NewRequest(http.MethodGet, "/components/status/test-component?debug=false", nil)
	w := httptest.NewRecorder()

	cm.handleComponentStatus("/components/status/")(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	_, hasDebug := response["debug"]
	assert.False(t, hasDebug, "Response should not have debug field when debug=false")
}

// TestHandleComponentStatus_DebugParamTrue_ProviderExists: debug=true adds what a
// component.DebugStatusProvider reports.
func TestHandleComponentStatus_DebugParamTrue_ProviderExists(t *testing.T) {
	cm := newDebugStatusComponentManager(t)

	req := httptest.NewRequest(http.MethodGet, "/components/status/test-component?debug=true", nil)
	w := httptest.NewRecorder()

	cm.handleComponentStatus("/components/status/")(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	debugField, hasDebug := response["debug"]
	require.True(t, hasDebug, "Response should have debug field when debug=true")
	require.NotNil(t, debugField, "Debug field should not be nil")

	debugMap, ok := debugField.(map[string]any)
	require.True(t, ok, "Debug field should be a map")
	assert.Equal(t, "test-data", debugMap["test_field"])
}

// TestHandleComponentStatus_DebugParamTrue_NoProvider: debug=true on a component that is not a
// component.DebugStatusProvider is still a 200, with no debug field.
func TestHandleComponentStatus_DebugParamTrue_NoProvider(t *testing.T) {
	cm := newNonDebugStatusComponentManager(t)

	req := httptest.NewRequest(http.MethodGet, "/components/status/non-debug-component?debug=true", nil)
	w := httptest.NewRecorder()

	cm.handleComponentStatus("/components/status/")(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, "non-debug-component", response["name"])

	_, hasDebug := response["debug"]
	assert.False(t, hasDebug, "Response should not have debug field when component doesn't implement DebugStatusProvider")
}

// TestHandleComponentStatus_DebugParamInvalid: only the exact value "true" asks for debug.
func TestHandleComponentStatus_DebugParamInvalid(t *testing.T) {
	cm := newDebugStatusComponentManager(t)

	testCases := []struct {
		name        string
		queryParam  string
		expectDebug bool
		description string
	}{
		{
			name:        "invalid value - not boolean",
			queryParam:  "debug=invalid",
			expectDebug: false,
			description: "Invalid boolean value should be treated as false",
		},
		{
			name:        "empty value",
			queryParam:  "debug=",
			expectDebug: false,
			description: "Empty debug parameter should be treated as false",
		},
		{
			name:        "numeric value",
			queryParam:  "debug=1",
			expectDebug: false,
			description: "Numeric value should be treated as false (not truthy)",
		},
		{
			name:        "uppercase TRUE",
			queryParam:  "debug=TRUE",
			expectDebug: false,
			description: "Case-sensitive: TRUE should not equal true",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/components/status/test-component?"+tc.queryParam, nil)
			w := httptest.NewRecorder()

			cm.handleComponentStatus("/components/status/")(w, req)

			require.Equal(t, http.StatusOK, w.Code, "Invalid param should not cause error")

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			_, hasDebug := response["debug"]
			assert.Equal(t, tc.expectDebug, hasDebug, tc.description)
		})
	}
}

// TestHandleComponentStatus_DebugFieldJSONSerializable: the status with its debug field is valid
// JSON and encodes again.
func TestHandleComponentStatus_DebugFieldJSONSerializable(t *testing.T) {
	cm := newDebugStatusComponentManager(t)

	req := httptest.NewRequest(http.MethodGet, "/components/status/test-component?debug=true", nil)
	w := httptest.NewRecorder()

	cm.handleComponentStatus("/components/status/")(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response), "Response should be valid JSON")

	_, err := json.Marshal(response)
	require.NoError(t, err, "Response with debug field should be re-serializable")
}

// TestHandleComponentStatus_ComponentNotFound: an unknown name is a 404, debug or not.
func TestHandleComponentStatus_ComponentNotFound(t *testing.T) {
	cm := newDebugStatusComponentManager(t)

	req := httptest.NewRequest(http.MethodGet, "/components/status/nonexistent?debug=true", nil)
	w := httptest.NewRecorder()

	cm.handleComponentStatus("/components/status/")(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

// TestHandleComponentStatus_MethodNotAllowed: the status route answers GET only.
func TestHandleComponentStatus_MethodNotAllowed(t *testing.T) {
	cm := newDebugStatusComponentManager(t)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/components/status/test-component?debug=true", nil)
			w := httptest.NewRecorder()

			cm.handleComponentStatus("/components/status/")(w, req)

			require.Equal(t, http.StatusMethodNotAllowed, w.Code)
		})
	}
}

// TestExtractComponentNameRefusesUnsafeNames: the name is the whole of the path after the route the
// handler was registered under (a prefix holding a route word included, #366), read from the
// request httptest.NewRequest parses (its Path already decoded, the encoded form kept in RawPath),
// as a handler receives it. A name that is empty, "." or "..", has a second segment, or decodes to
// a path separator, is refused, which the status route answers with a 400 (design D5; #365). It
// decodes with path semantics, so "+" stays "+".
func TestExtractComponentNameRefusesUnsafeNames(t *testing.T) {
	cases := []struct {
		route string
		path  string
		name  string
		ok    bool
	}{
		{route: "/components/status/", path: "/components/status/test-component", name: "test-component", ok: true},
		{route: "/components/status/", path: "/components/status/test-component/", name: "test-component", ok: true},
		{route: "/components/status/", path: "/components/status/a+b", name: "a+b", ok: true},
		{route: "/components/config/", path: "/components/config/test-component", name: "test-component", ok: true},
		{route: "/components/types/", path: "/components/types/udp", name: "udp", ok: true},
		{
			route: "/api/status/components/status/",
			path:  "/api/status/components/status/test-component",
			name:  "test-component",
			ok:    true,
		},
		{route: "/components/status/", path: "/components/status/"},
		{route: "/components/status/", path: "/components/status//"},
		{route: "/components/status/", path: "/components/status/test-component//"},
		{route: "/components/status/", path: "/components/status/."},
		{route: "/components/status/", path: "/components/status/.."},
		{route: "/components/status/", path: "/components/status/%2E%2E"},
		{route: "/components/status/", path: "/components/status/test%2Fcomponent"},
		{route: "/components/status/", path: "/components/status/unknown%2Ftest-component"},
		{route: "/components/status/", path: "/components/status/unknown/test-component"},
		{route: "/components/status/", path: "/components/status/test%5Ccomponent"},
		{route: "/components/status/", path: `/components/status/test\component`},
		{route: "/components/status/", path: "/components/config/test-component"},
	}
	for _, tc := range cases {
		request := httptest.NewRequest(http.MethodGet, tc.path, nil)
		name, ok := extractComponentName(request, tc.route)
		if name != tc.name || ok != tc.ok {
			t.Errorf("extractComponentName(%q, %q) = %q, %v; want %q, %v", tc.path, tc.route, name, ok, tc.name, tc.ok)
		}
	}

	cm := newDebugStatusComponentManager(t)
	for _, target := range []string{"/components/status/..", "/components/status/test%5Ccomponent"} {
		w := httptest.NewRecorder()
		cm.handleComponentStatus("/components/status/")(w, httptest.NewRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want %d", target, w.Code, http.StatusBadRequest)
		}
	}
}

// TestComponentRoutesRejectMalformedName: through the registered mux, a name that is empty, has
// more than one segment, or decodes to a separator answers 400 on the status, config and type
// routes, even where its last segment names a composed component, so a wrong lookup cannot hide
// behind a 404 (design D5; #365).
func TestComponentRoutesRejectMalformedName(t *testing.T) {
	cm := newDebugStatusComponentManager(t)
	mux := http.NewServeMux()
	cm.RegisterHTTPHandlers("/components", mux)

	control := httptest.NewRecorder()
	mux.ServeHTTP(control, httptest.NewRequest(http.MethodGet, "/components/status/test-component", nil))
	require.Equal(t, http.StatusOK, control.Code, "the target component must exist: %s", control.Body.String())

	for _, path := range []string{
		"/components/status/unknown%2Ftest-component",
		"/components/config/unknown%2Ftest-component",
		"/components/types/unknown%2Ftest-component",
		"/components/status/unknown/test-component",
		"/components/config/unknown/test-component",
		"/components/status/",
		"/components/config/",
		"/components/types/",
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s: got %d, want 400: %s", path, w.Code, w.Body.String())
		}
	}
}

// TestComponentRoutesRespectTheirMountPrefix: a name is read after the exact prefix the routes were
// registered under, so a prefix that itself holds a route word still serves a valid name (#366).
func TestComponentRoutesRespectTheirMountPrefix(t *testing.T) {
	cm := newDebugStatusComponentManager(t)
	for _, route := range []string{"status", "config", "types"} {
		prefix := "/api/" + route + "/components"
		mux := http.NewServeMux()
		cm.RegisterHTTPHandlers(prefix, mux)
		path := prefix + "/" + route + "/test-component"
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200: %s", path, w.Code, w.Body.String())
		}
	}
}

// TestFlowValidationHandlerProjectsLibraryResult: <components>/validate serves the result kept at
// construction verbatim. The pin handed a struct literal a result whose status its arrays could
// not derive; a manager built through NewComponentManager keeps the library's own analysis, so
// the test asserts the served body is that kept result and, independently, the finding the
// composition library documents for a component with no connections.
func TestFlowValidationHandlerProjectsLibraryResult(t *testing.T) {
	cm := newTestComponentManager(t, testComponent{name: "alone", instance: &mockDiscoverableComponent{}})

	recorder := httptest.NewRecorder()
	cm.handleFlowValidation(recorder, httptest.NewRequest(http.MethodGet, "/components/validate", nil))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var served composition.Result
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &served), recorder.Body.String())

	retained := cm.bootCompositionResult()
	require.NotNil(t, retained)
	if !reflect.DeepEqual(served, *retained) {
		t.Fatalf("served result is not the retained result verbatim:\nserved=%+v\nretained=%+v", served, *retained)
	}

	require.Equal(t, composition.StatusWarnings, served.Status)
	require.Empty(t, served.Errors)
	require.Len(t, served.Warnings, 1)
	require.Equal(t, composition.TypeDisconnectedNode, served.Warnings[0].Type)
	require.Equal(t, "alone", served.Warnings[0].Component)
	require.Len(t, served.Graph.Nodes, 1)
	require.Equal(t, "alone", served.Graph.Nodes[0].Instance)
}

// TestFlowGraphMatchesGoldenForFixtureComposition: the flowgraph answer is a tested contract
// (design D5; docs/contract.md). For a fixture composition of an input, a processor and an
// output, joined by a JetStream stream, a core NATS subject and a KV bucket, the JSON and the
// Mermaid answers of the route RegisterHTTPHandlers serves are the files under testdata/. The
// files were written from the composition library's analysis and Mermaid rendering of the same
// declarations, and read by hand; a change to either answer is a change to the contract and
// updates the file deliberately.
func TestFlowGraphMatchesGoldenForFixtureComposition(t *testing.T) {
	cm := newProjectionComponentManager(t, component.NewRegistry(), flowGraphFixtureComposition()...)
	mux := http.NewServeMux()
	cm.RegisterHTTPHandlers("/components", mux)

	wantJSON, err := os.ReadFile(filepath.Join("testdata", "flowgraph_fixture.golden.json"))
	require.NoError(t, err)
	wantMermaid, err := os.ReadFile(filepath.Join("testdata", "flowgraph_fixture.golden.mmd"))
	require.NoError(t, err)

	t.Run("JSON", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/components/flowgraph", nil))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

		// The file is the served body indented, so a difference reads line by line.
		var got bytes.Buffer
		require.NoError(t, json.Indent(&got, bytes.TrimSpace(recorder.Body.Bytes()), "", "  "))
		got.WriteByte('\n')
		require.Equal(t, string(wantJSON), got.String())
	})

	t.Run("Mermaid", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/components/flowgraph?format=mermaid", nil))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, "text/plain; charset=utf-8", recorder.Header().Get("Content-Type"))
		require.Equal(t, string(wantMermaid), recorder.Body.String())
	})

	// The two files describe one graph: the Mermaid file is the library's rendering of the JSON
	// file, so neither can drift from the other unnoticed.
	t.Run("FilesAgree", func(t *testing.T) {
		var graph composition.Graph
		require.NoError(t, json.Unmarshal(wantJSON, &graph))
		require.Equal(t, string(wantMermaid), composition.Mermaid(graph))
	})
}

// flowGraphFixtureComposition is the golden test's composition: source publishes raw events on
// the EVENTS stream, enricher consumes them and publishes enriched events on a core NATS subject
// and its state in the ENRICHED_STATE bucket, and sink reads both.
func flowGraphFixtureComposition() []projectionComponent {
	return []projectionComponent{
		{name: "source", componentType: types.ComponentTypeInput, instance: &portFactsDiscoverable{
			outputs: []component.Port{
				{Name: "raw", Direction: component.DirectionOutput, Description: "raw events",
					Config: component.JetStreamPort{StreamName: "EVENTS", Subjects: []string{"events.raw"}}},
			},
		}},
		{name: "enricher", componentType: types.ComponentTypeProcessor, instance: &portFactsDiscoverable{
			inputs: []component.Port{
				{Name: "raw", Direction: component.DirectionInput, Required: true,
					Config: component.JetStreamPort{StreamName: "EVENTS", Subjects: []string{"events.raw"}}},
			},
			outputs: []component.Port{
				{Name: "enriched", Direction: component.DirectionOutput,
					Config: component.NATSPort{Subject: "events.enriched"}},
				{Name: "state", Direction: component.DirectionOutput,
					Config: component.KVWritePort{Bucket: "ENRICHED_STATE"}},
			},
		}},
		{name: "sink", componentType: types.ComponentTypeOutput, instance: &portFactsDiscoverable{
			inputs: []component.Port{
				{Name: "enriched", Direction: component.DirectionInput,
					Config: component.NATSPort{Subject: "events.enriched"}},
				{Name: "state", Direction: component.DirectionInput,
					Config: component.KVReadPort{Bucket: "ENRICHED_STATE"}},
			},
		}},
	}
}

// newDebugStatusComponentManager builds a manager composing test-component, a
// component.DebugStatusProvider reporting {"test_field": "test-data"}.
func newDebugStatusComponentManager(t *testing.T) *ComponentManager {
	t.Helper()
	return newTestComponentManager(t, testComponent{
		name:     "test-component",
		instance: &mockComponentWithDebug{debugData: map[string]any{"test_field": "test-data"}},
	})
}

// newNonDebugStatusComponentManager builds a manager composing non-debug-component, which is not a
// component.DebugStatusProvider.
func newNonDebugStatusComponentManager(t *testing.T) *ComponentManager {
	t.Helper()
	return newTestComponentManager(t, testComponent{
		name: "non-debug-component", instance: &mockDiscoverableComponent{},
	})
}

// mockComponentWithDebug is a component.DebugStatusProvider reporting debugData.
type mockComponentWithDebug struct {
	baseDiscoverable
	debugData any
}

func (m *mockComponentWithDebug) DebugStatus() any { return m.debugData }
