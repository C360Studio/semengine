package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/types"
)

// The retired /gaps operation and the component manager's OpenAPI document, from the pin's
// component_manager_gaps_removed_test.go, plus TestOpenAPIDocumentResolvesEverySchemaRef (design
// D5, option (a)). The pin built a struct-literal manager and ran analyzeBootComposition by hand;
// here NewComponentManager admits the component and analyses the composition once, as at boot
// (design D3, D7).

// externalOrphanFixture is one admitted component whose only input is a
// required JetStream port fed from outside the composition. Under ADR-100 the
// canonical judgment raises no orphan for it; the retired /gaps operation
// classified the same port as a critical orphan and set has_issues=true.
func externalOrphanFixture(t *testing.T) *ComponentManager {
	t.Helper()
	instance := &portFactsDiscoverable{
		inputs: []component.Port{{
			Name:      "user.message",
			Direction: component.DirectionInput,
			Required:  true,
			External:  true,
			Config: component.JetStreamPort{
				StreamName: "USER_MESSAGES",
				Subjects:   []string{"user.message.>"},
			},
		}},
	}
	return newProjectionComponentManager(t, component.NewRegistry(), projectionComponent{
		name: "consumer", componentType: types.ComponentTypeProcessor, instance: instance,
	})
}

// TestComponentGapsOperationIsAbsent proves the second composition judgment is
// gone (ADR-100 D3, one validator and one vocabulary): the /gaps route is not
// served and the ComponentManager OpenAPI document does not advertise it.
func TestComponentGapsOperationIsAbsent(t *testing.T) {
	mux := http.NewServeMux()
	newProjectionComponentManager(t, component.NewRegistry()).RegisterHTTPHandlers("/components/", mux)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(method, "/components/gaps", nil))
		if recorder.Code != http.StatusNotFound && recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s /components/gaps = %d, want 404 or 405", method, recorder.Code)
		}
	}

	if _, advertised := componentManagerOpenAPISpec().Paths["/gaps"]; advertised {
		t.Fatal("the ComponentManager OpenAPI document still advertises /gaps")
	}
}

// TestExternalInputIsNeverACriticalOrphanOnAnyComponentOperation enumerates the
// component surface from its owning OpenAPI declaration rather than from a
// hand-kept list, so a reintroduced second judgment is caught wherever it is
// mounted. Every advertised GET operation answers 200, and none reports the
// externally fed required input as an orphan, a publisher-less port, or an
// issue count.
func TestExternalInputIsNeverACriticalOrphanOnAnyComponentOperation(t *testing.T) {
	manager := externalOrphanFixture(t)
	mux := http.NewServeMux()
	manager.RegisterHTTPHandlers("/components/", mux)

	paths := componentManagerOpenAPISpec().Paths
	if len(paths) == 0 {
		t.Fatal("the ComponentManager OpenAPI document advertises no operation")
	}
	// A judgment vocabulary the canonical library does not own. "critical" and
	// "has_issues" were the retired handler's severity words; "no_publishers"
	// and "orphaned_port" are the classifications it applied to this port.
	banned := []string{"no_publishers", "orphaned_port", "critical", "has_issues"}

	// The pin exempted status/{name} and config/{name} from answering, because its
	// struct-literal fixture held no component record. A manager NewComponentManager
	// built holds the consumer's record and its factory, so every GET answers here.
	for path, item := range paths {
		concrete := strings.NewReplacer("{name}", "consumer", "{id}", "consumer").Replace(path)
		for _, method := range declaredMethods(item) {
			t.Run(method+" "+path, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				mux.ServeHTTP(recorder, httptest.NewRequest(method, "/components"+concrete, nil))
				if method == http.MethodGet && recorder.Code != http.StatusOK {
					t.Fatalf("%s /components%s = %d, want %d: %s",
						method, concrete, recorder.Code, http.StatusOK, recorder.Body.String())
				}
				if recorder.Code >= http.StatusInternalServerError {
					t.Fatalf("%s /components%s = %d: %s", method, concrete, recorder.Code, recorder.Body.String())
				}
				body := recorder.Body.String()
				for _, word := range banned {
					if strings.Contains(body, word) {
						t.Fatalf("%s /components%s reports %q for an external input: %s",
							method, concrete, word, body)
					}
				}
			})
		}
	}

	// The canonical judgment agrees: no error finding, and the port carries its
	// marker in the projection.
	result := manager.bootCompositionResult()
	if result == nil || len(result.Errors) != 0 {
		t.Fatalf("boot composition result = %+v, want a result with no error finding", result)
	}
	if len(result.Graph.Nodes) != 1 {
		t.Fatalf("projection nodes = %+v, want the one admitted instance", result.Graph.Nodes)
	}
	node := result.Graph.Nodes[0]
	if len(node.Inputs) != 1 || !node.Inputs[0].External {
		t.Fatalf("projection input = %+v, want the external marker", node.Inputs)
	}
}

// declaredMethods returns every HTTP method the path item declares, so a future
// non-GET operation on the component surface is searched too rather than
// silently skipped by a GET-only walk.
func declaredMethods(item PathSpec) []string {
	methods := make([]string, 0, 5)
	for method, operation := range map[string]*OperationSpec{
		http.MethodGet:    item.GET,
		http.MethodPost:   item.POST,
		http.MethodPut:    item.PUT,
		http.MethodPatch:  item.PATCH,
		http.MethodDelete: item.DELETE,
	} {
		if operation != nil {
			methods = append(methods, method)
		}
	}
	slices.Sort(methods)
	return methods
}

// TestOpenAPIDocumentResolvesEverySchemaRef: a composition whose service registry RegisterAll
// filled, started with StartAll, serves an /openapi.json in which every $ref names a schema the
// same document carries, the component manager's Graph and Result among them. /openapi.json takes
// its schemas only from the process-wide OpenAPI registry, so the refs resolve only because
// RegisterAll registered the component manager's spec there; importing the package registers
// nothing (design D5, option (a); sweep C9).
//
// spec: operator-surface / The component manager's routes are served under /components
func TestOpenAPIDocumentResolvesEverySchemaRef(t *testing.T) {
	// The OpenAPI registry is process-wide and other tests call RegisterAll too: forget any
	// earlier registration of the component manager's spec, so the one this test reads is the
	// RegisterAll call below, and put the registry back as it was when the test ends.
	openAPIRegistryMu.Lock()
	previous, registered := openAPIRegistry[componentManagerName]
	delete(openAPIRegistry, componentManagerName)
	openAPIRegistryMu.Unlock()
	t.Cleanup(func() {
		openAPIRegistryMu.Lock()
		defer openAPIRegistryMu.Unlock()
		delete(openAPIRegistry, componentManagerName)
		if registered {
			openAPIRegistry[componentManagerName] = previous
		}
	})

	manager := createTestServiceManager(ManagerConfig{}, testComponentDependencies(t, component.NewRegistry()))
	require.NoError(t, RegisterAll(manager.registry))
	startTestManager(t, manager)

	status, body, err := httpGet(t.Context(), observedManagerHTTPURL(t, manager)+"/openapi.json")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, body)
	var document struct {
		Paths      map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &document))
	require.Contains(t, document.Paths, "/components/flowgraph",
		"the document does not list the component manager's paths under /components")

	var whole any
	require.NoError(t, json.Unmarshal([]byte(body), &whole))
	refs := openAPIDocumentRefs(whole)
	require.Subset(t, refs, []string{"#/components/schemas/Graph", "#/components/schemas/Result"},
		"the component manager's flowgraph and validate responses are not referenced")
	for _, ref := range refs {
		name, local := strings.CutPrefix(ref, "#/components/schemas/")
		require.True(t, local, "$ref %q does not point into the document's schemas", ref)
		require.Contains(t, document.Components.Schemas, name, "$ref %q names no schema in the document", ref)
	}
}

// openAPIDocumentRefs returns every "$ref" string value anywhere in a decoded JSON document,
// sorted and without repeats.
func openAPIDocumentRefs(value any) []string {
	var refs []string
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for key, child := range v {
				if ref, ok := child.(string); ok && key == "$ref" {
					refs = append(refs, ref)
					continue
				}
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(value)
	slices.Sort(refs)
	return slices.Compact(refs)
}
