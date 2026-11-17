package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/c360studio/semengine/graph"
)

// searchEntity is what a search reads of an entity: its ID and, for its label, its triples. An
// object is kept as its JSON, so a value the search does not read, such as a large integer, is
// never converted.
type searchEntity struct {
	ID      string `json:"id"`
	Triples []struct {
		Predicate string          `json:"predicate"`
		Object    json.RawMessage `json:"object"`
	} `json:"triples"`
}

func decodeSearchEntity(raw json.RawMessage) (searchEntity, error) {
	var entity searchEntity
	if err := json.Unmarshal(raw, &entity); err != nil {
		return entity, fmt.Errorf("entity does not decode: %w", err)
	}
	return entity, nil
}

// label is the first non-empty string value, in triple order, of the first of predicates the entity
// holds one for: predicates is the label set, most salient first (labelOrder). An entity with none
// has no label.
func (e searchEntity) label(predicates []string) string {
	for _, predicate := range predicates {
		for _, triple := range e.Triples {
			var value string
			if triple.Predicate == predicate && json.Unmarshal(triple.Object, &value) == nil && value != "" {
				return value
			}
		}
	}
	return ""
}

// wordSearch asks the word index for request and returns its reply. A refusal reaches the caller as
// it arrived; a reply that does not decode, names no producer or holds no results list is graph-query's
// own refusal, counted.
func (c *Component) wordSearch(ctx context.Context, lexicalSubject string, request graph.LexicalSearchRequest) (graph.QueryResponse[graph.LexicalSearchData], error) {
	data, err := json.Marshal(request)
	if err != nil {
		return graph.QueryResponse[graph.LexicalSearchData]{}, internalError(err)
	}
	raw, err := c.ask(ctx, lexicalSubject, data)
	if err != nil {
		return graph.QueryResponse[graph.LexicalSearchData]{}, err
	}
	reply, err := decodeIndexReply[graph.LexicalSearchData](c, lexicalSubject, raw)
	if err != nil {
		return reply, err
	}
	if reply.Data.Results == nil {
		return reply, c.brokenReply(lexicalSubject, errors.New("holds no results"))
	}
	return reply, nil
}

// batchReply is the batch verb's reply with each entity kept as the bytes it sent, so graph-query
// passes an entity on without decoding and encoding it again.
type batchReply struct {
	Entities []json.RawMessage     `json:"entities"`
	Missing  []graph.MissingEntity `json:"missing"`
}

// readGaps is what readMatches left out of its matches: the number of hits the batch named missing
// for any reason but damaged, and the hits it named damaged, in the hits' order.
type readGaps struct {
	missing int
	damaged []string
}

// left reports whether any hit was left out.
func (g readGaps) left() bool { return g.missing > 0 || len(g.damaged) > 0 }

// readMatches reads hits back with one batch request and returns them as matches, in the hits'
// order, each labelled from labels. A hit the batch names damaged is left out and returned in
// damaged, in the hits' order: its stored value cannot be read, and the other hits still answer the
// search. Any other hit the batch names missing is left out and counted in missing: an entity can be
// deleted between the word index's answer and the read. No hits make no request.
// A batch refusal reaches the caller as it arrived. A reply that does not decode, holds an entity
// that does not decode, or neither returns nor names missing a hit, is graph-query's own refusal,
// counted: answering past it would drop a match without saying so.
func (c *Component) readMatches(ctx context.Context, batchSubject string, hits []graph.LexicalHit, labels []string) ([]graph.TextSearchMatch, readGaps, error) {
	matches := []graph.TextSearchMatch{}
	if len(hits) == 0 {
		return matches, readGaps{}, nil
	}
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.EntityID)
	}
	request, err := json.Marshal(graph.EntityBatchRequest{IDs: ids})
	if err != nil {
		return nil, readGaps{}, internalError(err)
	}
	raw, err := c.ask(ctx, batchSubject, request)
	if err != nil {
		return nil, readGaps{}, err
	}
	var reply batchReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, readGaps{}, c.brokenReply(batchSubject, err)
	}
	read := make(map[string]json.RawMessage, len(reply.Entities))
	entities := make(map[string]searchEntity, len(reply.Entities))
	for _, entityRaw := range reply.Entities {
		entity, err := decodeSearchEntity(entityRaw)
		if err != nil {
			return nil, readGaps{}, c.brokenReply(batchSubject, err)
		}
		if _, seen := read[entity.ID]; !seen {
			read[entity.ID] = entityRaw
			entities[entity.ID] = entity
		}
	}
	gone := make(map[string]graph.MissingReason, len(reply.Missing))
	for _, missing := range reply.Missing {
		gone[missing.ID] = missing.Reason
	}
	var gaps readGaps
	for _, hit := range hits {
		reason, named := gone[hit.EntityID]
		entityRaw, ok := read[hit.EntityID]
		switch {
		case ok:
			matches = append(matches, graph.TextSearchMatch{
				ID: hit.EntityID, Label: entities[hit.EntityID].label(labels), Score: hit.Score, Entity: entityRaw,
			})
		case named && reason == graph.MissingDamaged:
			gaps.damaged = append(gaps.damaged, hit.EntityID)
		case named:
			gaps.missing++
		default:
			return nil, readGaps{}, c.brokenReply(batchSubject, fmt.Errorf("neither returns nor names missing %s", hit.EntityID))
		}
	}
	return matches, gaps, nil
}
