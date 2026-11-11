// Package graphgateway provides graph-gateway, a read-only GraphQL view of the graph. Its schema
// (schema.graphql) is the contract: a request is parsed and validated against it before anything is
// asked of the graph, it names one root field besides __typename, and its answer holds exactly the
// fields it selected, under their aliases and through fragments. A root field whose reply does not fit
// the schema answers null with an error naming the path; the rest of the answer stands.
package graphgateway
