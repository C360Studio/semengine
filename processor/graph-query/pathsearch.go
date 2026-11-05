package graphquery

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/vocabulary"
)

const (
	directionOutgoing = "outgoing"
	directionIncoming = "incoming"
	directionBoth     = "both"
)

// pathSearch answers the entities and edges within a request's reach (design D2, D3): it reads the
// start through the entity verb, then walks breadth first through the index. Counted as entityByAlias
// is: a request is one input however many requests it makes, and at most one error, since any failed
// read ends the walk.
func (c *Component) pathSearch(entitySubject, outgoingSubject, incomingSubject string) queryHandler {
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		request, err := decodePathSearchRequest(data)
		if err != nil {
			return nil, invalidRequest(err)
		}
		if err := c.readPathStart(ctx, entitySubject, request.StartEntity); err != nil {
			return nil, err
		}
		found, revision, err := walkPaths(ctx, request, c.indexHop(outgoingSubject, incomingSubject, request.MinRevision))
		if err != nil {
			return nil, err
		}
		return envelope(c, outgoingSubject, revision, found)
	}
}

// decodePathSearchRequest decodes strictly and checks every field, before any downstream request.
func decodePathSearchRequest(data []byte) (graph.PathSearchRequest, error) {
	var request graph.PathSearchRequest
	if err := decodeStrict(data, &request); err != nil {
		return request, err
	}
	if err := semtypes.ValidateEntityID(request.StartEntity); err != nil {
		return request, fmt.Errorf("start_entity: %w", err)
	}
	switch request.Direction {
	case directionOutgoing, directionIncoming, directionBoth:
	default:
		return request, fmt.Errorf("direction %q is not outgoing, incoming or both", request.Direction)
	}
	if request.MaxDepth < 1 || request.MaxDepth > graph.MaxPathSearchDepth {
		return request, fmt.Errorf("max_depth %d is outside 1 to %d", request.MaxDepth, graph.MaxPathSearchDepth)
	}
	if request.MaxNodes < 0 || request.MaxNodes > graph.MaxPathSearchNodes {
		return request, fmt.Errorf("max_nodes %d is outside 0 to %d", request.MaxNodes, graph.MaxPathSearchNodes)
	}
	for i, predicate := range request.Predicates {
		if _, err := vocabulary.ParsePredicate(predicate); err != nil {
			return request, fmt.Errorf("predicates[%d]: %w", i, err)
		}
	}
	return request, nil
}

// readPathStart reads the start through the entity verb, so a start with no record is that verb's
// entity_not_found and not an empty answer. Its refusal reaches the caller as it arrived. Only whether
// the reply holds an entity is read here: its stored value is not this verb's to judge.
func (c *Component) readPathStart(ctx context.Context, subject, entityID string) error {
	request, err := json.Marshal(graph.EntityRequest{ID: entityID})
	if err != nil {
		return internalError(err)
	}
	raw, err := c.ask(ctx, subject, request)
	if err != nil {
		return err
	}
	var reply struct {
		Entity json.RawMessage `json:"entity"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return c.brokenReply(subject, err)
	}
	if len(reply.Entity) == 0 || string(reply.Entity) == "null" {
		return c.brokenReply(subject, errors.New("holds no entity"))
	}
	return nil
}

// pathHop reads one entity's edges in one direction, outgoing or incoming, as the index holds them,
// with the revision they were read at.
type pathHop func(ctx context.Context, entityID, direction string) ([]graph.RelationshipEdge, uint64, error)

// indexHop is the walk's hop in production: the index's outgoing or incoming verb, carrying
// minRevision. Whatever ends the hop keeps its class, code and detail, and its message names the hop's
// entity, so a caller can find a hub too large to answer (design D3).
func (c *Component) indexHop(outgoingSubject, incomingSubject string, minRevision uint64) pathHop {
	return func(ctx context.Context, entityID, direction string) ([]graph.RelationshipEdge, uint64, error) {
		request, err := json.Marshal(struct {
			EntityID string `json:"entity_id"`
			graph.MinRevisionField
		}{entityID, graph.MinRevisionField{MinRevision: minRevision}})
		if err != nil {
			return nil, 0, internalError(err)
		}
		var edges []graph.RelationshipEdge
		var revision uint64
		if direction == directionOutgoing {
			edges, revision, err = c.outgoingRows(ctx, outgoingSubject, entityID, request)
		} else {
			edges, revision, err = c.incomingRows(ctx, incomingSubject, entityID, request)
		}
		if err != nil {
			return nil, 0, namingHop(entityID, err)
		}
		return edges, revision, nil
	}
}

// namingHop adds the hop's entity to err's message and keeps the rest of its refusal.
func namingHop(entityID string, err error) error {
	refusal := &errs.ClassifiedError{Class: errs.ErrorTransient, Code: graph.ErrorCodeInternal}
	errors.As(err, &refusal)
	return errs.ClassifiedCodeDetail(refusal.Class, refusal.Code, refusal.Detail, fmt.Errorf("path search hop at %s: %w", entityID, err))
}

// walkPaths walks breadth first from the start, which the caller has read, hopping at each entity
// below max_depth, and returns what it found and the smallest revision any hop answered at. Any hop's
// failure ends the walk with no partial answer.
//
// Filling the limit does not end the walk: truncated must be exact, so the walk goes on reading the
// entities it holds, and stops at the first unseen entity it has no room for. Each entity is read at
// most once per direction, which is the bound on requests (design D4).
func walkPaths(ctx context.Context, request graph.PathSearchRequest, hop pathHop) (graph.PathSearchData, uint64, error) {
	limit := request.MaxNodes
	if limit == 0 {
		limit = graph.DefaultPathSearchNodes
	}
	found := graph.PathSearchData{
		Entities: []graph.PathEntity{{EntityID: request.StartEntity, Depth: 0}},
		Edges:    []graph.RelationshipEdge{},
	}
	seen := map[string]bool{request.StartEntity: true}
	smallest := uint64(math.MaxUint64)
	// The list grows as the loop reads it, in depth order, so the first entity at max_depth ends it.
	for next := 0; next < len(found.Entities) && found.Entities[next].Depth < request.MaxDepth; next++ {
		at := found.Entities[next]
		crossings, revision, err := neighbours(ctx, at.EntityID, request, hop)
		if err != nil {
			return graph.PathSearchData{}, 0, err
		}
		smallest = min(smallest, revision)
		for _, crossing := range crossings {
			if seen[crossing.neighbour] {
				continue
			}
			if len(found.Entities) == limit {
				found.Truncated = true
				return found, smallest, nil
			}
			seen[crossing.neighbour] = true
			found.Entities = append(found.Entities, graph.PathEntity{EntityID: crossing.neighbour, Depth: at.Depth + 1})
			found.Edges = append(found.Edges, crossing.edge)
		}
	}
	return found, smallest, nil
}

// pathCrossing is one edge a walk can cross from an entity, to neighbour.
type pathCrossing struct {
	neighbour string
	incoming  bool
	edge      graph.RelationshipEdge
}

// neighbours reads an entity's edges in the request's direction, filtered and sorted by entity,
// predicate, then outgoing before incoming, so a truncated answer repeats against an unchanged graph.
// Repeats need no removal of their own: the walk takes the first of any neighbour. It returns the
// smaller revision of the two legs of both.
func neighbours(ctx context.Context, entityID string, request graph.PathSearchRequest, hop pathHop) ([]pathCrossing, uint64, error) {
	var crossings []pathCrossing
	smallest := uint64(math.MaxUint64)
	read := func(direction string) error {
		edges, revision, err := hop(ctx, entityID, direction)
		if err != nil {
			return err
		}
		smallest = min(smallest, revision)
		for _, edge := range edges {
			if len(request.Predicates) > 0 && !slices.Contains(request.Predicates, edge.Predicate) {
				continue
			}
			crossing := pathCrossing{neighbour: edge.ToEntityID, edge: edge}
			if direction == directionIncoming {
				crossing.neighbour, crossing.incoming = edge.FromEntityID, true
			}
			crossings = append(crossings, crossing)
		}
		return nil
	}
	if request.Direction != directionIncoming {
		if err := read(directionOutgoing); err != nil {
			return nil, 0, err
		}
	}
	if request.Direction != directionOutgoing {
		if err := read(directionIncoming); err != nil {
			return nil, 0, err
		}
	}
	slices.SortFunc(crossings, func(a, b pathCrossing) int {
		return cmp.Or(
			strings.Compare(a.neighbour, b.neighbour),
			strings.Compare(a.edge.Predicate, b.edge.Predicate),
			compareBool(a.incoming, b.incoming),
		)
	})
	return crossings, smallest, nil
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case b:
		return -1
	default:
		return 1
	}
}
