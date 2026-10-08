package graphingest

import (
	"fmt"
	"slices"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

// The reasons the stream lane refuses a statement, each the reason label it is counted
// under on predicate_contract_rejections_total (design D15, statement metadata).
const (
	statementReasonMissingSource    = "missing_source"
	statementReasonMissingTimestamp = "missing_timestamp"
	statementReasonReservedSource   = "reserved_source"
)

// statementRefusal is a Graphable payload's statement the stream lane refuses: it lacks a
// Source or Timestamp the message envelope lacks too, or, once stamped, it carries a reserved
// source (graph.IsReservedSource). Such a message can never be admitted, so the consume
// closure terminates, counts and logs it as processIngest does a structurally invalid
// candidate.
type statementRefusal struct {
	index  int
	field  string // "source" or "timestamp"
	reason string // one of the statementReason constants
	source string // the reserved source, when reason is statementReasonReservedSource
}

func (r *statementRefusal) Error() string {
	if r.reason == statementReasonReservedSource {
		return fmt.Sprintf("triple[%d] source %q is reserved: graph-ingest and the lifecycle manager write under it", r.index, r.source)
	}
	return fmt.Sprintf("triple[%d] has no %s and the message envelope has none", r.index, r.field)
}

// stampFromEnvelope returns triples with each empty Source taken from the envelope's source
// and each zero Timestamp from its creation time (design D15, #98); a statement that carries
// its own keeps it. It copies before the first stamp, so the payload's slice is never
// written. It refuses as invalid_request, naming the statement's index, a statement whose
// missing value the envelope lacks too, and a statement whose source, once stamped, is
// reserved; nothing of a refused message is stored.
func stampFromEnvelope(triples []message.Triple, meta message.Meta) ([]message.Triple, error) {
	var envelopeSource string
	var envelopeTime time.Time
	if meta != nil {
		envelopeSource = meta.Source()
		envelopeTime = meta.CreatedAt()
	}
	var stamped []message.Triple // nil until the first stamp copies triples
	for index, triple := range triples {
		needsSource, needsTime := triple.Source == "", triple.Timestamp.IsZero()
		if needsSource {
			if envelopeSource == "" {
				return nil, refuseStatement(&statementRefusal{index: index, field: "source", reason: statementReasonMissingSource})
			}
			triple.Source = envelopeSource
		}
		if needsTime {
			if envelopeTime.IsZero() {
				return nil, refuseStatement(&statementRefusal{index: index, field: "timestamp", reason: statementReasonMissingTimestamp})
			}
			triple.Timestamp = envelopeTime
		}
		if graph.IsReservedSource(triple.Source) {
			return nil, refuseStatement(&statementRefusal{index: index, field: "source", reason: statementReasonReservedSource, source: triple.Source})
		}
		if needsSource || needsTime {
			if stamped == nil {
				stamped = slices.Clone(triples)
			}
			stamped[index] = triple
		}
	}
	if stamped == nil {
		return triples, nil
	}
	return stamped, nil
}

func refuseStatement(refusal *statementRefusal) error {
	return errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, refusal)
}
