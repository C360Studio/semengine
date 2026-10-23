// Package service provides OpenAPI specification types for HTTP endpoint documentation
package service

import (
	"cmp"
	"encoding/json"
	"net/http"
)

// HTTPHandler is an optional interface for services that want to expose HTTP endpoints
type HTTPHandler interface {
	RegisterHTTPHandlers(prefix string, mux *http.ServeMux)
	OpenAPISpec() *OpenAPISpec // Returns OpenAPI specification for this service
}

// ComponentsSpec holds reusable OpenAPI component definitions
type ComponentsSpec struct {
	Schemas map[string]any `json:"schemas,omitempty"`
}

// OpenAPIDocument represents the complete OpenAPI 3.0 specification
type OpenAPIDocument struct {
	OpenAPI    string              `json:"openapi"`
	Info       InfoSpec            `json:"info"`
	Servers    []ServerSpec        `json:"servers"`
	Paths      map[string]PathSpec `json:"paths"`
	Components *ComponentsSpec     `json:"components,omitempty"`
	Tags       []TagSpec           `json:"tags,omitempty"`
}

// MarshalJSON writes the document in OpenAPI 3.0's shape. Each operation's
// RequestBodySpec becomes requestBody, and its request body and each response
// with a schema or content type become content[mediaType].schema: a $ref, or an
// array of $ref items when IsArray is set. A request body, or a response with a
// schema, that names no content type is application/json; a content type with
// no schema has a media type with no schema. The service descriptors' own JSON
// is unchanged.
func (doc OpenAPIDocument) MarshalJSON() ([]byte, error) {
	type alias OpenAPIDocument
	paths := make(map[string]openAPIPathItem, len(doc.Paths))
	for path, spec := range doc.Paths {
		paths[path] = openAPIPathItem{
			GET:    newOpenAPIOperation(spec.GET),
			POST:   newOpenAPIOperation(spec.POST),
			PUT:    newOpenAPIOperation(spec.PUT),
			PATCH:  newOpenAPIOperation(spec.PATCH),
			DELETE: newOpenAPIOperation(spec.DELETE),
		}
	}
	// The outer Paths field hides the alias's.
	return json.Marshal(struct {
		alias
		Paths map[string]openAPIPathItem `json:"paths"`
	}{alias: alias(doc), Paths: paths})
}

// openAPIPathItem, openAPIOperation, openAPIRequestBody, openAPIResponse,
// openAPIMediaType and openAPISchema are OpenAPI 3.0's Path Item, Operation,
// Request Body, Response, Media Type and Schema objects, as far as the service
// descriptors fill them.
type openAPIPathItem struct {
	GET    *openAPIOperation `json:"get,omitempty"`
	POST   *openAPIOperation `json:"post,omitempty"`
	PUT    *openAPIOperation `json:"put,omitempty"`
	PATCH  *openAPIOperation `json:"patch,omitempty"`
	DELETE *openAPIOperation `json:"delete,omitempty"`
}

type openAPIOperation struct {
	Summary     string                     `json:"summary"`
	Description string                     `json:"description,omitempty"`
	Parameters  []ParameterSpec            `json:"parameters,omitempty"`
	RequestBody *openAPIRequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]openAPIResponse `json:"responses"`
	Tags        []string                   `json:"tags,omitempty"`
}

type openAPIRequestBody struct {
	Description string                      `json:"description,omitempty"`
	Required    bool                        `json:"required,omitempty"`
	Content     map[string]openAPIMediaType `json:"content"`
}

type openAPIResponse struct {
	Description string                      `json:"description"`
	Content     map[string]openAPIMediaType `json:"content,omitempty"`
}

type openAPIMediaType struct {
	Schema *openAPISchema `json:"schema,omitempty"`
}

type openAPISchema struct {
	Ref   string         `json:"$ref,omitempty"`
	Type  string         `json:"type,omitempty"`
	Items *openAPISchema `json:"items,omitempty"`
}

func newOpenAPIOperation(op *OperationSpec) *openAPIOperation {
	if op == nil {
		return nil
	}
	out := &openAPIOperation{
		Summary:     op.Summary,
		Description: op.Description,
		Parameters:  op.Parameters,
		Responses:   make(map[string]openAPIResponse, len(op.Responses)),
		Tags:        op.Tags,
	}
	if body := op.RequestBody; body != nil {
		out.RequestBody = &openAPIRequestBody{
			Description: body.Description,
			Required:    body.Required,
			Content:     openAPIContent(cmp.Or(body.ContentType, "application/json"), body.SchemaRef, false),
		}
	}
	for code, resp := range op.Responses {
		response := openAPIResponse{Description: resp.Description}
		if resp.ContentType != "" || resp.SchemaRef != "" {
			response.Content = openAPIContent(cmp.Or(resp.ContentType, "application/json"), resp.SchemaRef, resp.IsArray)
		}
		out.Responses[code] = response
	}
	return out
}

// openAPIContent returns a content map holding contentType, whose schema is
// schemaRef, an array of schemaRef items when isArray is set, or none when
// schemaRef is empty.
func openAPIContent(contentType, schemaRef string, isArray bool) map[string]openAPIMediaType {
	var media openAPIMediaType
	switch {
	case schemaRef == "":
	case isArray:
		media.Schema = &openAPISchema{Type: "array", Items: &openAPISchema{Ref: schemaRef}}
	default:
		media.Schema = &openAPISchema{Ref: schemaRef}
	}
	return map[string]openAPIMediaType{contentType: media}
}
