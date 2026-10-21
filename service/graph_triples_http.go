package service

// graph_triples_http.go — operator-facing read endpoint for graph triples,
// served on the service manager's mux, which every deployment has.
//
// Operator-facing read endpoint for graph triples; expect low-throughput use
// (e2e tests, ops dashboards, debugging). Not a high-volume API.
// Authentication is delegated to whatever gates the service-manager mux —
// no auth model changes in this file.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/vocabulary"
)

const (
	graphTriplesDefaultLimit = 100
	graphTriplesMaxLimit     = 1000

	// graphTriplesQueryTimeout is the per-request deadline for the NATS scan.
	// Low-throughput operator endpoint — 10 s is generous.
	graphTriplesQueryTimeout = 10 * time.Second
)

// tripleQueryParams holds parsed and validated query parameters.
// An empty string for subject, predicate, or object means "don't filter this
// field" — it acts as a wildcard and matches any value.
type tripleQueryParams struct {
	subject   string // empty = match any; else a literal entity ID
	predicate string // empty = match any; else a three-part predicate
	object    string // empty = match any; compared as fmt.Sprintf("%v", triple.Object)
	limit     int
}

// parseTripleQueryParams parses and validates GET /graph/triples query parameters.
// Returns (params, errMsg) where errMsg is non-empty on bad input and names the
// parameter.
//
// A subject must be a literal entity ID (types.ValidateEntityID) and a predicate
// a three-part predicate (vocabulary.ParsePredicate), so a malformed filter is
// refused rather than answered with an empty match.
//
// limit semantics:
//   - omitted or 0 → default 100
//   - negative      → 400 Bad Request
//   - > 1000        → clamped to 1000
func parseTripleQueryParams(r *http.Request) (tripleQueryParams, string) {
	q := r.URL.Query()

	p := tripleQueryParams{
		subject:   q.Get("subject"),
		predicate: q.Get("predicate"),
		object:    q.Get("object"),
		limit:     graphTriplesDefaultLimit,
	}

	if p.subject != "" {
		if err := types.ValidateEntityID(p.subject); err != nil {
			return tripleQueryParams{}, fmt.Sprintf("invalid subject %q: %v", p.subject, err)
		}
	}
	if p.predicate != "" {
		if _, err := vocabulary.ParsePredicate(p.predicate); err != nil {
			return tripleQueryParams{}, fmt.Sprintf("invalid predicate %q: %v", p.predicate, err)
		}
	}

	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return tripleQueryParams{}, fmt.Sprintf("invalid limit %q: must be an integer", raw)
		}
		if n < 0 {
			return tripleQueryParams{}, fmt.Sprintf("invalid limit %d: must be non-negative", n)
		}
		if n == 0 {
			// limit=0 treated as "use default"
			n = graphTriplesDefaultLimit
		}
		if n > graphTriplesMaxLimit {
			n = graphTriplesMaxLimit
		}
		p.limit = n
	}

	return p, ""
}

// tripleMatchesQuery reports whether t satisfies all non-empty filter fields.
// An empty filter field is a wildcard — it matches any value.
func tripleMatchesQuery(t message.Triple, p tripleQueryParams) bool {
	if p.subject != "" && t.Subject != p.subject {
		return false
	}
	if p.predicate != "" && t.Predicate != p.predicate {
		return false
	}
	if p.object != "" {
		// Object is typed as any; compare its string representation.
		if fmt.Sprintf("%v", t.Object) != p.object {
			return false
		}
	}
	return true
}

// handleGraphTriples serves GET /graph/triples from ENTITY_STATES.
func (m *Manager) handleGraphTriples(w http.ResponseWriter, r *http.Request) {
	serveGraphTriples(w, r, m.queryGraphTriples, m.logger)
}

// queryGraphTriples scans ENTITY_STATES for the triples params selects. With no
// NATS client it answers an empty list, not an error.
func (m *Manager) queryGraphTriples(ctx context.Context, params tripleQueryParams) ([]message.Triple, error) {
	if m.natsClient == nil {
		return []message.Triple{}, nil
	}
	reader, err := kvcatalog.OpenCatalogReader(ctx, m.natsClient, graph.BucketEntityStates)
	if err != nil {
		return nil, fmt.Errorf("open %s bucket: %w", graph.BucketEntityStates, err)
	}
	return scanGraphTriples(ctx, reader, params, m.logger)
}

// serveGraphTriples answers GET /graph/triples with the triples querier returns,
// as JSON. The service manager's handler passes its ENTITY_STATES query.
//
// Query params (all optional); an empty or omitted param means "don't filter
// this field" — it matches any value, not the literal empty string:
//
//	subject    - exact match on triple subject field; a literal entity ID
//	predicate  - exact match on triple predicate field; a three-part predicate
//	object     - string representation match on triple object field
//	limit      - max results (default 100, clamped to 1000; 0 uses default)
//
// Returns:
//
//	200 + JSON array of message.Triple objects (empty array on no matches, never null)
//	400 + text error naming the parameter on malformed query params
//	405 + text error on non-GET methods
//	500 + text error on backend failure
func serveGraphTriples(
	w http.ResponseWriter,
	r *http.Request,
	querier func(ctx context.Context, params tripleQueryParams) ([]message.Triple, error),
	logger *slog.Logger,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	params, errMsg := parseTripleQueryParams(r)
	if errMsg != "" {
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), graphTriplesQueryTimeout)
	defer cancel()

	results, err := querier(ctx, params)
	if err != nil {
		if logger != nil {
			logger.Error("graph triples query failed", slog.Any("error", err))
		}
		http.Error(w, "backend query failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil && logger != nil {
		logger.Error("failed to encode graph triples response", slog.Any("error", err))
	}
}

// scanGraphTriples reads ENTITY_STATES through reader and returns the first
// params.limit triples matching params, in entity-ID order and then in each
// entity's triple order. The keys are sorted before the scan, so the answer is
// the same however the bucket lists them. It returns an empty (non-nil) slice
// when nothing matches.
//
// A key deleted between the listing and its read is skipped: the entity no
// longer exists, so leaving it out is the answer, not a degrade. Any other read
// error, and an entity graph.UnmarshalEntityState refuses, fails the whole scan
// with nil triples, never the triples read so far.
//
// Scanning all entities for a predicate filter is acceptable for low-throughput
// operator use (e2e tests, dashboards). For high-volume use, a predicate index
// would be required — outside current scope.
func scanGraphTriples(
	ctx context.Context,
	reader kvcatalog.CatalogReader,
	params tripleQueryParams,
	logger *slog.Logger,
) ([]message.Triple, error) {
	keys, err := graphTripleKeys(ctx, reader)
	if err != nil {
		return nil, fmt.Errorf("list entity keys: %w", err)
	}
	slices.Sort(keys)

	results := make([]message.Triple, 0)
	for _, key := range keys {
		if len(results) >= params.limit {
			break
		}

		entry, err := graphTripleEntry(ctx, reader, key)
		if errors.Is(err, natsclient.ErrKVKeyNotFound) {
			logger.Debug("graph triples: skipping key deleted after listing", slog.String("key", key))
			continue
		}
		if err != nil {
			return nil, err
		}

		var entity graph.EntityState
		if err := graph.UnmarshalEntityState(key, entry.Value(), &entity); err != nil {
			return nil, fmt.Errorf("decode authoritative entity %q: %w", key, err)
		}

		for _, t := range entity.Triples {
			if len(results) >= params.limit {
				break
			}
			if tripleMatchesQuery(t, params) {
				results = append(results, t)
			}
		}
	}

	return results, nil
}

func graphTripleKeys(ctx context.Context, reader kvcatalog.CatalogReader) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, natsclient.DefaultKVOptions().Timeout)
	defer cancel()

	keys, err := reader.Keys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("kv keys: %w", err)
	}
	return keys, nil
}

func graphTripleEntry(ctx context.Context, reader kvcatalog.CatalogReader, key string) (jetstream.KeyValueEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, natsclient.DefaultKVOptions().Timeout)
	defer cancel()

	entry, err := reader.Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return nil, natsclient.ErrKVKeyNotFound
		}
		return nil, fmt.Errorf("kv get %s: %w", key, err)
	}
	return entry, nil
}
