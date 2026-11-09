package fusion

// The fused response contract (ADR-062 lens-driven entry). Lifted from
// semsource's validated source/fusion contract.go — the response shape an agent
// acts on: verbatim body + the structure around it, keyed by what a human reads
// (never an entity ID), with provenance and an explicit Deferred flag so a caller
// can calibrate trust and never mistake "not ready" for "not found".

// ContractVersion identifies the wire shape of Request/Response.
//
// "2" removed the readiness envelope (`index`), the graph facet's `view_revision`, and
// the readiness gate's defer reasons: readiness is read from each reply (ADR-109 D1).
const ContractVersion = "2"

// Want enumerates the optional facets a caller can request. Empty defaults to
// body plus immediate relations.
type Want string

// The requestable facets.
const (
	WantBody      Want = "body"      // verbatim source/passage
	WantRelations Want = "relations" // callers/callees, links/sections
	WantPaths     Want = "paths"     // bounded outgoing relation paths from the seeds
	WantImpact    Want = "impact"    // transitive reverse-relation closure of the seeds
	WantGraph     Want = "graph"     // lossless structured projection: typed facts + directed edges + evidence
)

// Budget bounds a response. Zero fields take engine defaults.
type Budget struct {
	MaxNodes int `json:"max_nodes,omitempty"`
	MaxBytes int `json:"max_bytes,omitempty"`
}

// Request is the fused query, keyed by what an agent already knows — never an
// entity ID.
type Request struct {
	Query string `json:"query"`
	Want  []Want `json:"want,omitempty"`
	// Scope optionally constrains NL seed resolution to entities whose ID
	// matches at least one of these dot-delimited entity-ID prefixes
	// (OR-matched). Empty/absent means no filter — today's behavior. It lets a
	// lens instance over a shared embedding index retrieve only its own domain
	// so a smaller domain is not diluted by a larger co-resident one (ADR-071).
	// A list, not a scalar: one domain often spans several prefixes (semsource
	// "all code" = golang/python/ts/svelte). NL-only; ignored by symbol/prefix
	// resolve modes.
	Scope  []string `json:"scope,omitempty"`
	Budget Budget   `json:"budget,omitzero"`
	// IncludeScores asks for per-node rank and resolve similarity. Opt-in so the
	// default response is byte-unchanged: scores are debugging and calibration
	// signal, and most callers act on the ordering rather than the numbers.
	IncludeScores bool `json:"include_scores,omitempty"`
}

// Provenance records how an answer was produced so callers can calibrate trust.
type Provenance string

// The provenance tiers, in increasing order of uncertainty.
const (
	ProvenanceDeterministic Provenance = "deterministic" // exact lookup + structural walk
	ProvenanceEmbedding     Provenance = "embedding"     // seeds came from semantic search
	ProvenanceLLM           Provenance = "llm"           // an LLM reasoned over the result
)

// DeferReasonAllSeedsUnhydrated is the engine-level defer cause for a query whose seeds
// all resolved but none could be read: the reads answered and the hydration failed, so
// nothing was withheld for readiness. The other value Response.DeferReason takes is
// graph.ErrorCodeIndexNotReady.
const DeferReasonAllSeedsUnhydrated = "all_seeds_unhydrated"

// Ref points to a node by what a human reads — never the entity ID.
type Ref struct {
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Fragment string `json:"fragment,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// Node is one fused result: verbatim body plus the structure around it. Domains
// differ only in which roles populate Relations (code: callers/callees; docs:
// links/sections).
type Node struct {
	Name     string `json:"name"`
	Kind     string `json:"kind,omitempty"`
	Path     string `json:"path,omitempty"`
	Fragment string `json:"fragment,omitempty"`
	// Lines is [start,end] for code, nil for line-less domains (docs) — a slice
	// so omitempty actually omits it rather than emitting a spurious [0,0].
	Lines []int  `json:"lines,omitempty"`
	Body  string `json:"body,omitempty"`
	// BodyReason names why Body is absent when a body was requested (WantBody) but
	// could not be loaded — a bounded, closed reason (not_found / error) so a
	// missing body is a partial-result SIGNAL, never a silent empty string
	// (gh#616, #600). It is DISTINCT from Response.Unhydrated: a missing body is a
	// node that exists and ranks, so the reason rides on the node itself rather
	// than the top-level list. A failed body hydration never defers the response
	// or synthesizes a Miss — the node still ships.
	//
	// Omitted from the wire when the body hydrates, so a fully-hydrated response is
	// byte-unchanged for existing consumers.
	BodyReason BodyReason       `json:"body_reason,omitempty"`
	Relations  map[string][]Ref `json:"relations,omitempty"`
	// RelationsTruncated reports that Relations is missing a ref: a role had more
	// relations than its cap, a relations Neighbors read failed and was skipped,
	// or a relation's target read failed or found nothing. Omitted otherwise.
	RelationsTruncated bool `json:"relations_truncated,omitempty"`
	// Class is the BFO/CCO class IRI (provenance/debug; the agent ignores it).
	Class string `json:"class,omitempty"`
	// Handle is an opaque continuation token (internally the entity ID). Not an
	// addressing scheme: never parse or construct it.
	Handle string `json:"handle,omitempty"`
	// Rank is this node's 1-based position in RESOLVE order — where the index put it
	// before the engine re-ranked. Present only when the request set IncludeScores.
	//
	// Deliberately not the node's position in the response: that is the array index,
	// which the caller can already count. Ranking reorders (lexical, ontology,
	// salience), so the GAP between resolve rank and response position is the whole
	// signal — it is what makes "why did this come out third?" answerable.
	Rank int `json:"rank,omitempty"`
	// Similarity is the resolve mode's own relevance score, present only when
	// IncludeScores was set AND the mode reports one (semantic does; symbol and prefix
	// do not). Joined to this node by entity ID, never by position.
	//
	// A POINTER so the wire is self-describing: absent means the mode does not score,
	// present means this is the score — including a genuine 0.0, which a bare
	// `float64,omitempty` would erase into indistinguishability from "unavailable".
	// A separate has_similarity bool round-trips correctly in Go but forces every
	// non-Go consumer to learn that an absent key means zero rather than nothing.
	Similarity *float64 `json:"similarity,omitempty"`
}

// Miss reports a query that resolved to nothing, with near-matches.
//
// A Miss is NOT an absence proof and is no longer tied to Ready (ADR-084): it is
// reachable under ordinary lag now that healthy-but-behind indexes serve, and it was
// never evidence the entity does not exist — only that this lookup found nothing.
// Callers that must distinguish "found nothing" from "could not read" want Unhydrated,
// which is a statement about the read.
type Miss struct {
	Query      string   `json:"query"`
	DidYouMean []string `json:"did_you_mean,omitempty"`
}

// Response is the fused answer. Nodes is the payload; Provenance and Deferred are
// the honesty envelope. Paths, Impact, and Graph are optional facets, present
// only when the request Wants them (WantPaths / WantImpact / WantGraph).
type Response struct {
	Provenance Provenance `json:"provenance"`
	Nodes      []Node     `json:"nodes,omitempty"`
	Paths      []Path     `json:"paths,omitempty"`
	// PathsTruncated reports that Paths is missing a path: more paths existed than
	// the cap, or a path was not emitted because a node on it could not be read.
	// Omitted otherwise.
	PathsTruncated bool    `json:"paths_truncated,omitempty"`
	Impact         *Impact `json:"impact,omitempty"`
	// Graph is the structured graph projection (WantGraph): typed property
	// facts, explicit directed edges, and verbatim evidence, with its own caps
	// (its reads are independent, not one snapshot). Its truncation metadata is self-contained —
	// graph-facet truncation NEVER sets the top-level Truncated bit, and a
	// request without the graph want omits the field entirely (the v1 wire
	// shape is unchanged for non-requesting callers).
	Graph  *GraphProjection `json:"graph,omitempty"`
	Misses []Miss           `json:"misses,omitempty"`
	// Unhydrated names requested seeds that did not load — DISTINCT from Misses. A
	// Miss says "the graph was asked and had nothing"; an Unhydrated entry says "we
	// could not read this one", a statement about the read rather than about the
	// world. Neither licenses an absence conclusion (ADR-084), and the
	// all-seeds-unhydrated case deliberately synthesizes NO Miss: claiming a miss
	// there would assert exactly the thing the failed read left unknown.
	//
	// Omitted when everything hydrated, so a complete response is byte-unchanged.
	Unhydrated []Unhydrated `json:"unhydrated,omitempty"`
	// Deferred reports that the engine WITHHELD rather than answered: the empty
	// result is a refusal to look, not a finding. Check this to answer "did I get
	// an answer?".
	Deferred bool `json:"deferred,omitempty"`
	// DeferReason is why: graph.ErrorCodeIndexNotReady when a read's reply said the
	// index was not ready, or DeferReasonAllSeedsUnhydrated when seeds resolved and
	// none could be read. Empty when not deferred.
	DeferReason string `json:"defer_reason,omitempty"`
	// UnreadableRecords and FailedRecords are the natural-language search's counts
	// of records it could not read and records whose embedding failed, copied from
	// its reply onto every response built after it answered: nodes, a miss, or a
	// later deferral. A miss beside a non-zero count is not "nothing there". Each is
	// omitted when 0, and both always are after a symbol or prefix resolve.
	UnreadableRecords int    `json:"unreadable_records,omitempty"`
	FailedRecords     int    `json:"failed_records,omitempty"`
	Truncated         bool   `json:"truncated"`
	ContractVersion   string `json:"contract_version"`
}

const (
	defaultMaxNodes = 20
	defaultMaxBytes = 60000
)

// wantSet expands a Want slice into a lookup set, applying defaults when empty.
func wantSet(wants []Want) map[Want]bool {
	if len(wants) == 0 {
		return map[Want]bool{WantBody: true, WantRelations: true}
	}
	set := make(map[Want]bool, len(wants))
	for _, w := range wants {
		set[w] = true
	}
	return set
}

// budgeter accumulates nodes up to the request budget, reporting truncation.
type budgeter struct {
	maxNodes, maxBytes, nodes, bytes int
}

// newBudget builds a budgeter from a request budget, applying defaults.
func newBudget(b Budget) *budgeter {
	bg := &budgeter{maxNodes: b.MaxNodes, maxBytes: b.MaxBytes}
	if bg.maxNodes <= 0 {
		bg.maxNodes = defaultMaxNodes
	}
	if bg.maxBytes <= 0 {
		bg.maxBytes = defaultMaxBytes
	}
	return bg
}

// admit reports whether a node carrying bodyBytes still fits, updating totals.
// At least one node is always admitted so a single oversized node is not dropped.
func (b *budgeter) admit(bodyBytes int) bool {
	if b.nodes >= b.maxNodes {
		return false
	}
	if b.nodes > 0 && b.bytes+bodyBytes > b.maxBytes {
		return false
	}
	b.nodes++
	b.bytes += bodyBytes
	return true
}
