package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// routedService is a running, healthy Service that is also an HTTPHandler: the manager mounts
// what register adds under the service's prefix when it prepares the full route set, and lists
// spec in /openapi.json. A nil stop returns nil.
type routedService struct {
	MockService
	register func(prefix string, mux *http.ServeMux)
	spec     *OpenAPISpec
	stop     func(context.Context) error
}

func newRoutedService(name string, register func(prefix string, mux *http.ServeMux)) *routedService {
	return &routedService{
		MockService: MockService{name: name, status: StatusRunning, healthy: true},
		register:    register,
	}
}

func (s *routedService) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	s.register(prefix, mux)
}

func (s *routedService) OpenAPISpec() *OpenAPISpec { return s.spec }

func (s *routedService) Stop(ctx context.Context) error {
	if s.stop == nil {
		return nil
	}
	return s.stop(ctx)
}

// answer returns a handler that writes status and body.
func answer(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// count returns how many records the handler (pprof_test.go) kept.
func (h *recordingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

// TestOpenAPIDocumentListsMountedRoutes: after boot, /openapi.json is an OpenAPI 3.0.0 document
// whose paths are each HTTPHandler service's paths under its prefix, and /docs serves the
// Swagger UI page, which loads its scripts from unpkg.com, only when swagger_ui is true (design
// D6; the 03B operator-surface row's proof).
//
// spec: operator-surface / /openapi.json documents the mounted routes
func TestOpenAPIDocumentListsMountedRoutes(t *testing.T) {
	newWidgets := func() *routedService {
		widgets := newRoutedService("widgets", func(prefix string, mux *http.ServeMux) {
			mux.HandleFunc("GET "+prefix+"/items", answer(http.StatusOK, "[]"))
		})
		widgets.spec = &OpenAPISpec{Paths: map[string]PathSpec{
			"/items": {GET: &OperationSpec{
				Summary:   "List widgets",
				Responses: map[string]ResponseSpec{"200": {Description: "The widgets"}},
			}},
		}}
		return widgets
	}

	t.Run("swagger_ui true", func(t *testing.T) {
		manager := createTestServiceManager(ManagerConfig{SwaggerUI: true}, createTestServiceDependencies())
		require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
		require.NoError(t, manager.RegisterInstance("widgets", newWidgets()))
		startTestManager(t, manager)
		url := observedManagerHTTPURL(t, manager)

		status, body, err := httpGet(t.Context(), url+"/openapi.json")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, body)
		var document struct {
			OpenAPI string                     `json:"openapi"`
			Paths   map[string]json.RawMessage `json:"paths"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &document))
		require.Equal(t, "3.0.0", document.OpenAPI)
		require.Contains(t, document.Paths, "/widgets/items", "the service's path is not listed under its prefix")
		require.NotContains(t, document.Paths, "/items", "the service's path is listed without its prefix")

		status, body, err = httpGet(t.Context(), url+"/widgets/items")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, "the documented route is not mounted: %s", body)

		status, body, err = httpGet(t.Context(), url+"/docs")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, body)
		require.Contains(t, body, "unpkg.com")
	})

	t.Run("swagger_ui false", func(t *testing.T) {
		manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
		require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
		require.NoError(t, manager.RegisterInstance("widgets", newWidgets()))
		startTestManager(t, manager)
		url := observedManagerHTTPURL(t, manager)

		status, body, err := httpGet(t.Context(), url+"/openapi.json")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, body)

		status, _, err = httpGet(t.Context(), url+"/docs")
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, status, "/docs is served while swagger_ui is false")
	})
}

// TestOpenAPIDocumentSerialisesBodiesAsOpenAPI3: a service's declared request body and response
// schemas reach /openapi.json in OpenAPI 3.0's shape: requestBody and each response's
// content[mediaType].schema, a $ref, an array of $ref items for IsArray, and application/json when
// the request body names no content type. The expected JSON is written from the OpenAPI 3.0.0
// Operation, Request Body, Response and Media Type objects, not from the encoder (Codex review of
// PR #353, issue #357).
//
// spec: operator-surface / /openapi.json documents the mounted routes
func TestOpenAPIDocumentSerialisesBodiesAsOpenAPI3(t *testing.T) {
	widgets := newRoutedService("widgets", func(prefix string, mux *http.ServeMux) {
		mux.HandleFunc(prefix+"/items", answer(http.StatusOK, "[]"))
	})
	widgets.spec = &OpenAPISpec{Paths: map[string]PathSpec{
		"/items": {
			GET: &OperationSpec{
				Summary: "List widgets",
				Responses: map[string]ResponseSpec{
					"200": {Description: "The widgets", ContentType: "application/json",
						SchemaRef: "#/components/schemas/Widget", IsArray: true},
				},
			},
			POST: &OperationSpec{
				Summary: "Add a widget",
				RequestBody: &RequestBodySpec{Description: "The widget", Required: true,
					SchemaRef: "#/components/schemas/Widget"},
				Responses: map[string]ResponseSpec{
					"201": {Description: "Added", SchemaRef: "#/components/schemas/Widget"},
					"204": {Description: "Nothing to add"},
					"400": {Description: "Refused", ContentType: "text/plain"},
				},
			},
		},
	}}
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	require.NoError(t, manager.RegisterInstance("widgets", widgets))
	startTestManager(t, manager)

	status, body, err := httpGet(t.Context(), observedManagerHTTPURL(t, manager)+"/openapi.json")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, body)
	var document struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &document))
	require.Contains(t, document.Paths, "/widgets/items")
	require.JSONEq(t, `{
		"get": {
			"summary": "List widgets",
			"responses": {
				"200": {
					"description": "The widgets",
					"content": {"application/json": {"schema": {
						"type": "array", "items": {"$ref": "#/components/schemas/Widget"}}}}
				}
			}
		},
		"post": {
			"summary": "Add a widget",
			"requestBody": {
				"description": "The widget",
				"required": true,
				"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Widget"}}}
			},
			"responses": {
				"201": {
					"description": "Added",
					"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Widget"}}}
				},
				"204": {"description": "Nothing to add"},
				"400": {"description": "Refused", "content": {"text/plain": {}}}
			}
		}
	}`, string(document.Paths["/widgets/items"]))
}
