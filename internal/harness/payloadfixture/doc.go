// Package payloadfixture holds payload-registry fixtures for tests: an empty registry, a registry
// built from chosen registration functions, and a schema-less stub type. It lives in the harness,
// not in payloadregistry, because it imports testing and production code imports no test package.
package payloadfixture
