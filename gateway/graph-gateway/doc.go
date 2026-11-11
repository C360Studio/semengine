// Package graphgateway provides graph-gateway, a read-only GraphQL view of the graph. Its schema
// (schema.graphql) is the contract: a request is parsed and validated against it before anything is
// asked of the graph, it names one root field besides __typename, and its answer holds exactly the
// fields it selected, under their aliases and through fragments. A root field whose reply does not fit
// the schema answers null with an error naming the path; the rest of the answer stands.
//
// The component answers POST <prefix>/graphql, mounted by the service manager at /<instance>. Each
// root field asks one graph.query.* verb of graph.QueryVerbs: entity, prefix, predicate and
// pathSearch. A classified refusal answers HTTP 200 with its class and code in the error's
// extensions; a request with no responder answers 500 naming the subject, one past query_timeout_ms
// 504, and every request before Start or once Stop begins 503. Stop returns once no admitted request
// runs.
package graphgateway
