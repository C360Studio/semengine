// Package graphlexical is the graph-lexical component: it follows ENTITY_STATES and keeps an
// in-memory word index (graph/lexical) of each entity's text, rebuilt from the bucket on every
// start, and reports on the graph-lexical readiness key. An entity's text is chosen by
// graph/entitytext, the rule graph-embedding uses, split into the entity's name and its body. No
// model or external provider is involved at any tier.
//
// Every delivered change is applied in delivery order, and a delivery at or below the revision
// the index holds for its key is ignored. A revision is reported applied only once the index
// holds its text.
//
// An entity whose body is offloaded to a store is indexed by its inline text at once, and by its
// body once the one goroutine that reads every body has read it. A body the index does not hold
// is queued, unread, missing or has no store. Only unread is a failure: the readiness key reads
// degraded and Health names it until a repair pass, every 30 seconds, reads the body. A read that
// fails on a store skips that store until the next pass.
//
// A composition registers the factory with Register and creates the component through the
// component manager, with a configuration that Config describes.
package graphlexical
