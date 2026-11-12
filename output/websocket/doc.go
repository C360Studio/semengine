// Package websocket is the websocket output component: an HTTP server whose upgrade path streams
// frames to WebSocket clients. Each instance has exactly one input. A kv-watch of every
// ENTITY_STATES key streams applied state: a client receives one put frame per current entity,
// then a synced frame, then the changes after it. A core NATS subject streams its JSON messages as
// data frames, each delivered at most once, with nothing acknowledged or replayed. Every frame is
// one JSON text message; README.md is the wire format.
//
// Applied state converges rather than replays: a client that folds the frames it receives holds
// each entity's current state, but intermediate revisions of one key may be merged away. A client
// that stops reading never slows another: a subject client's oldest queued frame is dropped, an
// applied-state client's pending changes merge per key, and each is counted and logged.
//
// Every Origin is accepted and the output has no authentication, so it belongs behind the
// deployment's own network boundary. TLS comes from the deployment's server TLS configuration
// (file certificates; ACME is not supported).
//
// Start binds the listener and Stop is the only thing that ends it: ending Start's context
// afterwards stops nothing (ADR-113). A composition registers the factory with Register and creates
// the component through the component manager, with a configuration that README.md describes.
package websocket
