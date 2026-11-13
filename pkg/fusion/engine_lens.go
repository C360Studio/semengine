package fusion

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage"
)

// The lens-driven engine entry (ADR-062). Resolves a query against the graph
// through a Lens and returns a fused Response — the deterministic counterpart to
// research_graph's chain (same resolve→expand→assemble shape, no LLM). The
// resolve mode is a string, and verbatim bodies are hydrated by dereferencing a
// storage handle through the BodyResolver. Ranking is resolve order plus lexical
// name match, with ontology specificity and predicate salience added when
// RankSignals is attached.

// Engine bounds keep the multi-hop traversal cheap and predictable.
const (
	resolveLimit        = 40
	maxRelationsPerNode = 12
)

// Ranking weights. Resolve order (the graph's own index/semantic ranking) is the
// base signal; lexical name match reorders locally. Ontology specificity and
// predicate salience are SECONDARY reorderings folded in only when RankSignals
// is attached — deliberately small next to resolveScale (up to resolveScale×N)
// and lexExact so they break ties / nudge within a resolve-lexical tier rather
// than dominating the graph's own ranking.
const (
	lexExact      = 40.0
	lexPrefix     = 20.0
	lexContains   = 8.0
	resolveScale  = 4.0 // base score per resolve-rank position
	ontologyScale = 1.5 // per ontology-depth level (class specificity)
	salienceScale = 3.0 // per unit of an entity's net predicate salience (signed: boost − demotion)
)

// ontologyClassPredicate is the stamped BFO/CCO class triple (semsource ADR-0005).
// Read for Node.Class when present; no class is derived from the entity ID. Keep
// in sync with semsource's ontology.ClassPredicate: the two are separate
// constants, not one shared definition.
const ontologyClassPredicate = "entity.ontology.class"

// Engine runs the lens-driven deterministic fusion pipeline over a
// RetrievalClient, hydrating verbatim bodies through a BodyResolver. It is safe
// for concurrent Fuse calls: nothing in it changes after NewEngine.
type Engine struct {
	graph    RetrievalClient
	body     *BodyResolver
	logger   *slog.Logger
	signals  RankSignals             // optional; nil → resolve-order + lexical ranking only
	registry *metric.MetricsRegistry // optional; nil → no counters

	bodyFailures *prometheus.CounterVec // nil without a registry
	skippedReads *prometheus.CounterVec // nil without a registry
}

// NewEngine builds a lens-driven engine. graph and logger are required. body may
// be nil — a node whose lens yields a body handle then reports BodyError. A
// counter registration the registry refuses is returned, and no engine.
func NewEngine(graph RetrievalClient, body *BodyResolver, logger *slog.Logger, opts ...Option) (*Engine, error) {
	if graph == nil {
		return nil, errors.New("fusion: nil retrieval client")
	}
	if logger == nil {
		return nil, errors.New("fusion: nil logger")
	}
	e := &Engine{graph: graph, body: body, logger: logger}
	for _, opt := range opts {
		opt(e)
	}
	if err := e.registerCounters(); err != nil {
		return nil, fmt.Errorf("fusion: register counters: %w", err)
	}
	return e, nil
}

// Fuse resolves req against the graph through lens and returns the fused response.
//
// The honesty rule is that an empty result is never ambiguous. Fuse asks no
// readiness status; it reads readiness from each reply (ADR-109). Every failed
// read ends in one of these outcomes, checked in this order:
//
//   - the context ended: Fuse returns the context's error, whichever read failed.
//   - index_not_ready from Resolve, Entities or a relations Neighbors read: the
//     response is deferred (graph.ErrorCodeIndexNotReady), with no nodes and no miss.
//   - any other error from Resolve or Entities, fatal ones such as
//     graph_state_reset_required included: Fuse returns it.
//   - any other failed read (a relations Neighbors read, a relation target, the
//     paths, impact and graph walks, index_not_ready included for the walks): the
//     read is skipped and the answer goes on without it. A fatal walk read is
//     skipped too, because poison refuses only the poisoned entities (ADR-109)
//     and the reply cannot tell that refusal from a reset.
//
// A healthy graph that found nothing yields a miss with near-matches; seeds that
// resolved but could not be read yield Unhydrated, deferred, and NO miss.
func (e *Engine) Fuse(ctx context.Context, req Request, lens Lens) (Response, error) {
	if ctx == nil {
		return Response{}, errors.New("fusion: nil context")
	}
	skips := newSkipLog()
	defer e.report(skips)
	mode := lens.ResolveMode(req.Query)
	resolution, err := e.graph.Resolve(ctx, ResolveQuery{Query: req.Query, Mode: mode, Scope: req.Scope, Limit: resolveLimit})
	if err != nil {
		return seedReadFailure(ctx, err)
	}
	resp, err := e.fuseSeeds(ctx, req, lens, mode, resolution.Seeds, skips)
	if err != nil {
		return Response{}, err
	}
	// Every response built after a natural-language search answered carries what
	// the search could not read; symbol and prefix resolves count nothing, so
	// their responses carry neither count.
	if mode == ResolveModeNL {
		resp.UnreadableRecords, resp.FailedRecords = resolution.UnreadableRecords, resolution.FailedRecords
	}
	return resp, nil
}

// fuseSeeds hydrates, ranks and expands the resolved seeds into the response.
func (e *Engine) fuseSeeds(ctx context.Context, req Request, lens Lens, mode ResolveMode, seeds []Seed, skips *skipLog) (Response, error) {
	hydration, err := e.graph.Entities(ctx, SeedIDs(seeds))
	if err != nil {
		return seedReadFailure(ctx, err)
	}
	entities := hydration.Entities
	if len(entities) == 0 {
		// Nothing hydrated. If the seeds resolved but NONE of them loaded, this is a
		// hydration failure, not an absence: reporting it as a miss would claim the
		// graph looked and found nothing, which is exactly the conclusion it cannot
		// draw. Say what failed to hydrate and leave Misses empty.
		if len(hydration.Unhydrated) > 0 {
			// Nothing loaded and the caller gave up: the ended context is the
			// answer, as for any failed read.
			if cerr := ctx.Err(); cerr != nil {
				return Response{}, cerr
			}
			// Marked DEFERRED: nothing was read, so this is a withholding, not a
			// zero-result answer. The contract tells consumers to check Deferred to
			// answer "did I get an answer?", and leaving it false here would make the
			// documented rule wrong for the one case that most looks like an answer.
			resp := deferredResponse(DeferReasonAllSeedsUnhydrated)
			resp.Provenance = ProvenanceForMode(mode)
			resp.Unhydrated = hydration.Unhydrated
			return resp, nil
		}
		return e.miss(ctx, mode, req.Query)
	}

	names := make([]string, len(entities))
	for i, ent := range entities {
		names[i] = lens.Label(ent)
	}
	ranked := e.rankEntities(entities, names, req.Query)

	wants := wantSet(req.Want)
	resp := Response{Provenance: ProvenanceForMode(mode), ContractVersion: ContractVersion}
	resp.Unhydrated = hydration.Unhydrated
	nodes, truncated, err := e.buildNodes(ctx, ranked, lens, wants, newBudget(req.Budget), skips)
	if err != nil {
		return seedReadFailure(ctx, err)
	}
	resp.Nodes, resp.Truncated = nodes, truncated
	if req.IncludeScores {
		applyScores(resp.Nodes, ranked, seeds)
	}
	// Deliberately no `len(resp.Nodes) == 0 -> synthesize a Miss` here. It was
	// unreachable (entities non-empty implies ranked non-empty, and the budgeter always
	// admits the first node), and if it HAD fired it would have manufactured a bare
	// Miss with no near-matches — an absence claim from a budget accident, which is
	// exactly what ADR-084 forbids.
	// Optional facets, computed from the ranked seeds (not the display-budgeted
	// nodes) so they describe the query's structure, not the page. Each returns an
	// error only when the context ended.
	if wants[WantImpact] {
		if resp.Impact, err = e.computeImpact(ctx, ranked, lens, skips); err != nil {
			return Response{}, err
		}
	}
	if wants[WantPaths] {
		if resp.Paths, resp.PathsTruncated, err = e.computePaths(ctx, ranked, lens, skips); err != nil {
			return Response{}, err
		}
	}
	if wants[WantGraph] {
		if resp.Graph, err = e.computeGraph(ctx, ranked, lens, skips); err != nil {
			return Response{}, err
		}
	}
	return resp, nil
}

// seedReadFailure maps a failed Resolve, Entities, or relations Neighbors read to
// Fuse's result: an ended context fails the call with the context's error,
// index_not_ready defers, and anything else fails the call with the read's error.
func seedReadFailure(ctx context.Context, err error) (Response, error) {
	if cerr := ctx.Err(); cerr != nil {
		return Response{}, cerr
	}
	if isIndexNotReady(err) {
		return deferredResponse(graph.ErrorCodeIndexNotReady), nil
	}
	return Response{}, err
}

// applyScores attaches each node's RESOLVE rank and resolve similarity (ADR-084),
// opt-in via the request so the default wire shape is unchanged.
//
// Rank is the seed's 1-based position in RESOLVE order — deliberately not the node's
// position in the response, which the caller can already count off the array and which
// therefore carries no information. The whole point of the field is that rankEntities
// REORDERS (lexical affinity, ontology specificity, predicate salience), so the gap
// between where resolve put an entity and where it came out is the diagnostic signal:
// "why did this rank third when the index thought it was the best match?" is
// answerable only from the pair.
//
// Both values are joined BY ENTITY ID. Ranking reorders and the budget truncates, so a
// positional join would silently mislabel — the same class of bug as the cache-order
// scramble this change already fixed. nodes[i] corresponds to ranked[i] by construction
// (buildNodes appends in order and stops at the budget), so ranked supplies the join
// key; the key is what is trusted, not the index.
func applyScores(nodes []Node, ranked []*Entity, seeds []Seed) {
	type score struct {
		rank          int
		similarity    float64
		hasSimilarity bool
	}
	byID := make(map[string]score, len(seeds))
	for i, s := range seeds {
		byID[s.ID] = score{rank: i + 1, similarity: s.Similarity, hasSimilarity: s.HasSimilarity}
	}
	for i := range nodes {
		if i >= len(ranked) {
			// Defensive: buildNodes cannot produce more nodes than ranked entities.
			// Bailing beats indexing past the slice that supplies the join key.
			continue
		}
		sc, ok := byID[ranked[i].ID]
		if !ok {
			// A node whose entity was never a seed (not reachable today — nodes are
			// built from ranked seeds). Leaving rank unset beats inventing one.
			continue
		}
		nodes[i].Rank = sc.rank
		if sc.hasSimilarity {
			similarity := sc.similarity
			nodes[i].Similarity = &similarity
		}
	}
}

// isIndexNotReady reports whether err is the classified readiness transient a
// graph read emits while the index is catching up to ENTITY_STATES. It matches on
// the stable code (graph.ErrorCodeIndexNotReady), NOT errs.IsTransient — a real
// connection timeout is also transient but must propagate as a hard error, not be
// degraded to an empty envelope.
func isIndexNotReady(err error) bool {
	var ce *errs.ClassifiedError
	return errors.As(err, &ce) && ce.Code == graph.ErrorCodeIndexNotReady
}

// deferredResponse builds the empty response for every path that WITHHELD.
// One constructor so a new defer path cannot forget the flag — the failure mode being
// an empty response that a consumer reads as an answer.
func deferredResponse(reason string) Response {
	return Response{
		Provenance:      ProvenanceDeterministic,
		Deferred:        true,
		DeferReason:     reason,
		ContractVersion: ContractVersion,
	}
}

// miss builds a found-nothing response with near-matches. A failed Names read
// leaves the suggestions empty, which claims nothing; only an ended context fails
// the call.
func (e *Engine) miss(ctx context.Context, mode ResolveMode, query string) (Response, error) {
	names, err := e.graph.Names(ctx, query, 5)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return Response{}, cerr
		}
		names = nil
	}
	return Response{
		Provenance:      ProvenanceForMode(mode),
		Misses:          []Miss{{Query: query, DidYouMean: names}},
		ContractVersion: ContractVersion,
	}, nil
}

// buildNodes materializes ranked entities into Nodes within the budget. It
// returns the context's error, or the readiness transient a relations Neighbors
// read hit (via nodeFor → relations → collectEdges), so Fuse fails or defers the
// whole response rather than serving partial relations as ready.
func (e *Engine) buildNodes(ctx context.Context, ranked []*Entity, lens Lens, wants map[Want]bool, bg *budgeter, skips *skipLog) ([]Node, bool, error) {
	var nodes []Node
	for _, ent := range ranked {
		node, err := e.nodeFor(ctx, ent, lens, wants, skips)
		if err != nil {
			return nil, false, err
		}
		if !bg.admit(len(node.Body)) {
			return nodes, true, nil
		}
		nodes = append(nodes, node)
	}
	return nodes, false, nil
}

// nodeFor builds one Node, including only the requested facets. The body is
// hydrated by dereferencing the lens's Hydrate HANDLE through the BodyResolver
// (ADR-062). Hydration is best-effort and degrades-don't-fail: a Hydrate or
// deref failure omits the body and does NOT fail the node — but it is not
// SILENT: the node ships with a bounded BodyReason. Only an ended context fails
// it.
func (e *Engine) nodeFor(ctx context.Context, ent *Entity, lens Lens, wants map[Want]bool, skips *skipLog) (Node, error) {
	loc := lens.Location(ent)
	node := Node{
		Name:     lens.Label(ent),
		Kind:     lens.Kind(ent),
		Path:     loc.Path,
		Fragment: loc.Fragment,
		Lines:    lineRange(loc.Lines),
		Class:    classOf(ent),
		Handle:   ent.ID,
	}
	if wants[WantBody] {
		// Gate on the WANT alone, then always ask the lens for a handle. A nil BodyResolver
		// (NewEngine(graph, nil, logger)) is documented-valid, but it must not
		// SHORT-CIRCUIT the WantBody path — a node whose lens yields a real handle it
		// cannot resolve is a reportable config gap, which an empty body with no reason
		// would hide. The ref==nil case below still keeps genuinely body-less tiers silent,
		// so no resolver never spams BodyError for them.
		ref, err := lens.Hydrate(ctx, ent)
		switch {
		case err != nil:
			if cerr := ctx.Err(); cerr != nil {
				return Node{}, cerr
			}
			// A genuine retrieval fault producing the handle.
			e.reportBodyFailure(&node, BodyError, err)
		case ref == nil:
			// The entity legitimately has NO verbatim body (lens.go:171,177). This
			// is a normal condition, not a failure: leave Body empty and stamp no
			// reason.
		case e.body == nil:
			// The lens produced a REAL storage handle but no BodyResolver is wired to
			// dereference it. That is a configuration gap, NOT a body-less tier: the
			// node has a body it simply cannot load, so report it rather than ship an
			// unexplained empty body.
			// TODO(when lens Engine is wired): a distinct not_configured reason to
			// separate "no resolver wired" from "read faulted" (deferred — the
			// Engine has no production caller yet, so expanding the closed
			// BodyReason vocabulary for an unwired path is premature).
			e.reportBodyFailure(&node, BodyError, errors.New("fusion: no body resolver configured"))
		default:
			body, derr := e.body.ResolveBody(ctx, ref)
			switch {
			case derr == nil:
				node.Body = string(body)
			case ctx.Err() != nil:
				return Node{}, ctx.Err()
			case errors.Is(derr, storage.ErrObjectNotFound):
				// The ref is present on the entity but resolves to no stored object
				// — for example, content-addressed evidence was reclaimed while the
				// entity still points at it.
				e.reportBodyFailure(&node, BodyNotFound, derr)
			default:
				// The reference resolved to a store but the read faulted.
				e.reportBodyFailure(&node, BodyError, derr)
			}
		}
	}
	if wants[WantRelations] {
		rels, truncated, err := e.relations(ctx, ent, lens, skips)
		if err != nil {
			return Node{}, err
		}
		node.Relations, node.RelationsTruncated = rels, truncated
	}
	return node, nil
}

// reportBodyFailure stamps a body-hydration failure onto the node (bounded
// reason, empty body left as-is), logs it and counts it by reason. It never
// defers the response or synthesizes a Miss — the node is a partial result that
// still ships.
func (e *Engine) reportBodyFailure(node *Node, reason BodyReason, err error) {
	node.BodyReason = reason
	e.logger.Warn("fusion could not load a node body; the node ships without it",
		slog.String("handle", node.Handle), slog.String("reason", string(reason)), slog.Any("error", err))
	if e.bodyFailures != nil {
		e.bodyFailures.WithLabelValues(string(reason)).Inc()
	}
}

// relations expands a node's forward and reverse edges into role → refs, and
// reports whether a ref was left out: a role passed its cap, a Neighbors read was
// skipped, or a target's read was skipped or found nothing. It returns the
// context's error or the readiness transient a Neighbors read hit, so Fuse fails
// or defers.
func (e *Engine) relations(ctx context.Context, ent *Entity, lens Lens, skips *skipLog) (map[string][]Ref, bool, error) {
	preds, fwd, rev := edgePredicates(lens.Edges(), FacetRelations)
	if len(preds) == 0 {
		return nil, false, nil
	}
	rels := map[string][]Ref{}
	fwdTruncated, err := e.collectEdges(ctx, ent.ID, preds, Outgoing, fwd, lens, rels, skips)
	if err != nil {
		return nil, false, err
	}
	revTruncated, err := e.collectEdges(ctx, ent.ID, preds, Incoming, rev, lens, rels, skips)
	if err != nil {
		return nil, false, err
	}
	truncated := fwdTruncated || revTruncated
	if len(rels) == 0 {
		return nil, truncated, nil
	}
	return rels, truncated, nil
}

// collectEdges appends refs for one direction's edges, in (predicate, target)
// order and capped per role, and reports whether a ref was left out. It returns
// the context's error, or the readiness transient (graph.ErrorCodeIndexNotReady)
// so Fuse defers the whole response instead of serving partial-as-ready; any
// OTHER Neighbors fault, fatal included, skips this direction and is reported as
// a left-out ref.
func (e *Engine) collectEdges(ctx context.Context, id string, preds []string, dir Direction, roleByPred map[string]string, lens Lens, rels map[string][]Ref, skips *skipLog) (bool, error) {
	edges, err := e.graph.Neighbors(ctx, id, preds, dir)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return false, cerr
		}
		if isIndexNotReady(err) {
			return false, err
		}
		skips.add(string(WantRelations), err)
		return true, nil
	}
	truncated := false
	for _, ed := range orderedEdges(edges) {
		role := roleByPred[ed.Predicate]
		if role == "" {
			continue
		}
		if len(rels[role]) >= maxRelationsPerNode {
			truncated = true
			continue
		}
		ref, ok, err := e.refFor(ctx, ed.Target, lens)
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return false, cerr
			}
			skips.add(string(WantRelations), err)
		}
		if !ok {
			// Skipped or found nothing: either way the ref is left out.
			truncated = true
			continue
		}
		rels[role] = append(rels[role], ref)
	}
	return truncated, nil
}

// orderedEdges returns a copy of edges sorted by predicate, then target, with
// duplicates removed, so a cap keeps the same edges whatever order the reply
// listed them in (sort before limit).
func orderedEdges(edges []Edge) []Edge {
	out := slices.Clone(edges)
	slices.SortFunc(out, func(a, b Edge) int {
		if c := cmp.Compare(a.Predicate, b.Predicate); c != 0 {
			return c
		}
		return cmp.Compare(a.Target, b.Target)
	})
	return slices.Compact(out)
}

// refFor resolves a target entity ID to a human Ref. ok is false when the read
// found nothing; err is the read's error, which the caller skips unless the
// context ended.
func (e *Engine) refFor(ctx context.Context, id string, lens Lens) (Ref, bool, error) {
	te, err := e.graph.Entity(ctx, id)
	if err != nil {
		return Ref{}, false, err
	}
	if te == nil {
		return Ref{}, false, nil
	}
	loc := lens.Location(te)
	return Ref{Name: lens.Label(te), Path: loc.Path, Fragment: loc.Fragment, Line: loc.Lines[0]}, true, nil
}

// edgePredicates flattens a lens's EdgeSpecs into the predicate list and the
// forward/reverse role lookups used during expansion, restricted to the specs that
// participate in facet f. An EdgeSpec with no Facets participates in every facet
// (backward-compatible default); one with a non-empty Facets is included only for
// the facets it names.
func edgePredicates(specs []EdgeSpec, f Facet) (preds []string, fwd, rev map[string]string) {
	fwd = make(map[string]string, len(specs))
	rev = make(map[string]string, len(specs))
	for _, s := range specs {
		if !s.walksFacet(f) {
			continue
		}
		preds = append(preds, s.Predicate)
		if s.OutgoingRole != "" {
			fwd[s.Predicate] = s.OutgoingRole
		}
		if s.IncomingRole != "" {
			rev[s.Predicate] = s.IncomingRole
		}
	}
	return preds, fwd, rev
}

// classOf returns an entity's stamped BFO/CCO class IRI (entity.ontology.class), or "".
// No class is derived from the entity ID, and nothing ranks by ontology distance.
func classOf(e *Entity) string {
	return e.First(ontologyClassPredicate)
}

// lineRange renders a [start,end] pair as a wire slice, or nil for line-less
// domains so the field is omitted.
func lineRange(l [2]int) []int {
	if l[0] == 0 && l[1] == 0 {
		return nil
	}
	return []int{l[0], l[1]}
}

// rankEntities orders entities (given in resolve order) by resolve-rank base +
// lexical name match, plus — when RankSignals is attached — ontology
// specificity and predicate salience as secondary reorderings. names[i] is
// entity[i]'s display name. With no signals the result is exactly resolve order
// plus lexical match.
func (e *Engine) rankEntities(entities []*Entity, names []string, query string) []*Entity {
	q := strings.ToLower(strings.TrimSpace(query))
	type scored struct {
		e   *Entity
		s   float64
		pos int
	}
	arr := make([]scored, len(entities))
	for i, ent := range entities {
		s := resolveScale * float64(len(entities)-i)
		s += lexicalScore(q, strings.ToLower(names[i]))
		if e.signals != nil {
			s += ontologyScale * e.signals.ClassSpecificity(classOf(ent))
			s += salienceScale * entitySalience(ent, e.signals)
		}
		arr[i] = scored{ent, s, i}
	}
	sort.SliceStable(arr, func(a, b int) bool {
		if arr[a].s != arr[b].s {
			return arr[a].s > arr[b].s
		}
		return arr[a].pos < arr[b].pos
	})
	out := make([]*Entity, len(arr))
	for i := range arr {
		out[i] = arr[i].e
	}
	return out
}

// entitySalience is an entity's ranking salience: the entity's single most
// salient (max positive) weight PLUS its single strongest demotion (min
// negative) weight over its predicates — one presence-based boost and one
// presence-based penalty, summed. Extremes, not a sum over all predicates, so a
// densely-annotated entity does not outrank a sharply-identified one purely on
// fact count; and the two extremes come from different predicates (a
// predicate's weight is one sign), so a boost and a demotion compose without
// either masking the other.
//
// Signed weights are what let a ranker DEMOTE, not just boost: structurally-
// identifiable noise (tests, generated code, mocks) that carries the SAME
// salient predicates as the real thing — a *_test.go with the same doc-comment
// salience as its impl — also carries a negatively-weighted presence predicate
// (e.g. code.artifact.test → WithWeight(-1.5)), so it sinks below the impl,
// which a boost-only ranker could never separate it from. With no negative
// weights min-negative stays 0 and the salience is the strongest boost alone.
func entitySalience(ent *Entity, sig RankSignals) float64 {
	var maxPos, minNeg float64 // strongest boost (≥0), strongest demotion (≤0)
	for i := range ent.Triples {
		w := sig.PredicateSalience(ent.Triples[i].Predicate)
		if w > maxPos {
			maxPos = w
		}
		if w < minNeg {
			minNeg = w
		}
	}
	return maxPos + minNeg
}

// lexicalScore scores a name against a lowercased query.
func lexicalScore(q, name string) float64 {
	switch {
	case name == q:
		return lexExact
	case strings.HasPrefix(name, q):
		return lexPrefix
	case strings.Contains(name, q):
		return lexContains
	default:
		return 0
	}
}
