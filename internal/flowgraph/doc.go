// Package flowgraph builds a graph of how components connect through their
// declared ports and reports what that graph shows: which components are
// connected, which have no connections, and which ports nothing matches.
//
// # Building a graph
//
// [BuildFromDeclarations] builds a graph from the [component.Declaration] of
// every component instance (the port shape the component Registry records when
// it admits an instance, or derives offline from a configuration). It adds one
// node per instance and connects the ports. It returns an error when the
// declarations cannot form a valid graph: an empty or repeated instance name, a
// port with no connection identity, more than one writer port on a KV bucket,
// two exclusive binds on one network address, or a graph-mutation port that is
// malformed or not served by exactly one provider.
//
// In SemStreams the composition package is this package's one reader; it
// comes to SemEngine in a later change. Until then this package's tests are
// its only callers.
//
// # What the graph connects
//
// A port's pattern (its [component.InteractionPattern]) decides how it is
// matched.
//
//   - stream: an output joins every input on another component whose subject
//     matches. A JetStream port also matches on each of its stream's subjects.
//   - request: ports on different components with matching subjects join,
//     whatever their direction.
//   - watch and read: a watch output (a KV writer) joins every watch or read
//     input on the same bucket, its own component's included. A bucket with
//     more than one writer port is refused.
//   - store: every store output joins every store input on another component.
//   - network, http-client and timer: never joined. Two exclusive binds on one
//     network address are refused.
//
// Subject matching is this package's own rule: `*` stands for one token and
// `>` for one or more trailing tokens, and either side of a pair may hold the
// wildcard. [SubjectCovers] answers the one-way question of whether a filter
// covers every subject a pattern can name.
//
// # What the analysis reports
//
// [FlowGraph.AnalyzeConnectivity] reports the connected groups of components,
// the components with no edge, and the orphaned ports: ports no edge reaches.
// Network, http-client, store and timer ports are never orphans. Each orphan
// carries an Issue, one of the Issue constants.
//
// The analysis reports facts and no severity. Whether an orphan or a lone
// component is a problem is the reader's judgment.
//
// [FlowGraph.ValidateStreamRequirements] returns one warning for each
// JetStream input port that a plain NATS output port feeds, naming the
// publishing components. A JetStream consumer reads from a stream, and a plain
// NATS publisher does not create one.
//
// # Concurrency
//
// A FlowGraph has no locks. Build it and read it on one goroutine.
package flowgraph
