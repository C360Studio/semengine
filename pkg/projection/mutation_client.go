package projection

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

type mutationRequester interface {
	RequestClassified(context.Context, string, []byte, time.Duration) ([]byte, error)
}

type contractBinding struct {
	contract Contract
	groups   map[string]PredicateGroup
	allowed  map[string]struct{}
}

// MutationClient is an immutable, concurrency-safe graph mutation client.
type MutationClient struct {
	wire      *graphmutation.Client
	reader    *graph.ExactEntityReader
	contracts map[string]contractBinding
}

// NewMutationClient validates and copies the complete projection contract set.
func NewMutationClient(cfg MutationClientConfig) (*MutationClient, error) {
	if cfg.NATS == nil {
		return nil, errors.New("NATS client is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = natsclient.DefaultRequestTimeout
	}
	return newMutationClient(cfg.NATS, cfg.Contracts, timeout)
}

func newMutationClient(
	requester mutationRequester,
	contracts []Contract,
	timeout time.Duration,
) (*MutationClient, error) {
	bindings, err := buildContractIndex(contracts)
	if err != nil {
		return nil, err
	}
	wire, err := graphmutation.NewClient(requester, timeout)
	if err != nil {
		return nil, err
	}
	reader, err := graph.NewExactEntityReader(requester, timeout)
	if err != nil {
		return nil, err
	}
	return &MutationClient{
		wire:      wire,
		reader:    reader,
		contracts: bindings,
	}, nil
}

func buildContractIndex(contracts []Contract) (map[string]contractBinding, error) {
	if err := ValidateContracts(contracts); err != nil {
		return nil, err
	}
	index := make(map[string]contractBinding, len(contracts))
	for _, original := range contracts {
		contract := original.Clone()
		binding := contractBinding{
			contract: contract,
			groups:   make(map[string]PredicateGroup, len(contract.Groups)),
			allowed:  make(map[string]struct{}),
		}
		for _, predicate := range contract.BirthPredicates {
			binding.allowed[predicate] = struct{}{}
		}
		for _, group := range contract.Groups {
			binding.groups[group.Name] = group
			for _, predicate := range group.Predicates {
				binding.allowed[predicate] = struct{}{}
			}
		}
		index[contract.Name] = binding
	}
	return index, nil
}

func (c *MutationClient) binding(operation MutationOperation, name string) (contractBinding, error) {
	if c == nil {
		return contractBinding{}, invalidMutationError(operation, errors.New("mutation client is nil"))
	}
	binding, ok := c.contracts[name]
	if !ok {
		return contractBinding{}, invalidMutationError(operation, fmt.Errorf("unknown projection contract %q", name))
	}
	return binding, nil
}

// Create atomically creates an entity with its complete initial triples.
//
// A request that is incomplete or outside the contract is refused before it is sent: Kind
// MutationInvalid, not committed. Any other failure follows ClassifyMutationError. A nil error
// means the commit is verified.
func (c *MutationClient) Create(ctx context.Context, request CreateMutation) (MutationReceipt, error) {
	binding, err := c.binding(MutationOperationCreate, request.Contract)
	if err != nil {
		return MutationReceipt{}, err
	}
	if request.Entity == nil {
		return MutationReceipt{}, invalidMutationError(MutationOperationCreate, errors.New("entity is required"))
	}
	if len(request.Entity.Triples) != 0 {
		return MutationReceipt{}, invalidMutationError(MutationOperationCreate,
			errors.New("entity.triples must be empty; CreateMutation.Triples is the sole fact source"))
	}
	entity := request.Entity.Clone()
	// O-17: the bound contract is the only spelling of the type a caller needs.
	// A zero stamp is filled from the contract's structured type before
	// validation and before the request is built (no key is parsed); a
	// non-zero stamp that differs is rejected below.
	if entity.MessageType == (message.Type{}) && binding.contract.MessageType.IsValid() {
		entity.MessageType = binding.contract.MessageType
	}
	if err := validateEntity(binding.contract, entity); err != nil {
		return MutationReceipt{}, invalidMutationError(MutationOperationCreate, err)
	}
	triples, err := canonicalizeTriples(MutationOperationCreate, request.Triples, request.Metadata)
	if err != nil {
		return MutationReceipt{}, err
	}
	for index, triple := range triples {
		if triple.Subject != entity.ID {
			return MutationReceipt{}, invalidMutationError(MutationOperationCreate,
				fmt.Errorf("triple[%d] subject %q does not match entity %q", index, triple.Subject, entity.ID))
		}
		if _, allowed := binding.allowed[triple.Predicate]; !allowed {
			return MutationReceipt{}, invalidMutationError(MutationOperationCreate,
				fmt.Errorf("predicate %q is not declared by contract %q", triple.Predicate, binding.contract.Name))
		}
	}
	response, err := c.wire.Create(ctx, graph.CreateEntityRequest{
		Entity: entity, Triples: triples, IndexingProfile: binding.contract.IndexingProfile,
		TraceID: request.Metadata.TraceID, RequestID: request.Metadata.RequestID,
	})
	if err != nil {
		return mutationFailure(MutationOperationCreate, err)
	}
	return MutationReceipt{Entity: response.Entity.Clone(), KVRevision: response.KVRevision}, nil
}

// Reconcile replaces one complete reconcile-mode predicate group, at the request's
// ExpectedRevision or, when that is zero, at the revision the client reads.
//
// It reads the entity first. A failed read sends no mutation and is not committed, with operation
// read-authoritative; a stored entity outside the contract is refused as MutationInvalid.
// A request that is incomplete or outside the contract is refused before it is sent: Kind
// MutationInvalid, not committed. Any other failure follows ClassifyMutationError. A nil error
// means the commit is verified.
func (c *MutationClient) Reconcile(ctx context.Context, request ReconcileMutation) (MutationReceipt, error) {
	binding, group, desired, err := c.canonicalizeGroupMutation(
		MutationOperationReconcile, request.Contract, request.Group, request.EntityID, request.Desired, request.Metadata, ModeReconcile,
	)
	if err != nil {
		return MutationReceipt{}, err
	}
	// The read stays when the caller names a revision: graph-ingest's reconcile checks no
	// contract, so only this check holds the stored entity to the contract's message type.
	// Whether the caller's revision is still current is graph-ingest's to decide; the client
	// does not compare it with this read.
	exact, err := c.ReadAuthoritative(ctx, request.EntityID)
	if err != nil {
		return MutationReceipt{}, err
	}
	if err := validateEntity(binding.contract, exact.Entity); err != nil {
		return MutationReceipt{}, invalidMutationError(MutationOperationReconcile, err)
	}
	expected := request.ExpectedRevision
	if expected == 0 {
		expected = exact.KVRevision
	}
	response, err := c.wire.Reconcile(ctx, graph.ReconcilePredicatesRequest{
		EntityID: request.EntityID, ExpectedRevision: expected, Source: request.Metadata.Source,
		Predicates: append([]string(nil), group.Predicates...), Desired: desired,
		TraceID: request.Metadata.TraceID, RequestID: request.Metadata.RequestID,
	})
	if err != nil {
		return mutationFailure(MutationOperationReconcile, err)
	}
	return MutationReceipt{Entity: response.Entity.Clone(), KVRevision: response.KVRevision}, nil
}

// Append appends triples in one append-mode predicate group to one entity.
//
// A request that is incomplete or outside the contract is refused before it is sent: Kind
// MutationInvalid, not committed. Any other failure follows ClassifyMutationError. A nil error
// means the commit is verified.
func (c *MutationClient) Append(ctx context.Context, request AppendMutation) (MutationReceipt, error) {
	_, _, triples, err := c.canonicalizeGroupMutation(
		MutationOperationAppend, request.Contract, request.Group, request.EntityID, request.Triples, request.Metadata, ModeAppend,
	)
	if err != nil {
		return MutationReceipt{}, err
	}
	response, err := c.wire.Append(ctx, graph.AppendTriplesRequest{
		Triples: triples, TraceID: request.Metadata.TraceID, RequestID: request.Metadata.RequestID,
	})
	if err != nil {
		return mutationFailure(MutationOperationAppend, err)
	}
	// The wire client has refused any other outcome, so the rest is applied or unchanged.
	result := response.Results[0]
	switch result.Outcome {
	case graph.MutationFailed:
		return mutationFailure(MutationOperationAppend, classifiedAppendFailure(result.Error))
	case graph.MutationEntityNotFound:
		err := errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeEntityNotFound,
			fmt.Errorf("entity not found: %s", request.EntityID))
		return mutationFailure(MutationOperationAppend, err)
	}
	return MutationReceipt{KVRevision: result.KVRevision}, nil
}

// Delete conditionally deletes one entity. Callers supply the exact revision
// they intend to fence; the client does not hide a read or retry.
//
// A zero ExpectedRevision is refused before it is sent: Kind MutationInvalid, not committed. Any
// other failure follows ClassifyMutationError. A nil error means the commit is verified.
func (c *MutationClient) Delete(ctx context.Context, request DeleteMutation) (MutationReceipt, error) {
	if request.ExpectedRevision == 0 {
		return MutationReceipt{}, invalidMutationError(MutationOperationDelete, errors.New("expected revision must be nonzero"))
	}
	response, err := c.wire.Delete(ctx, graph.DeleteEntityRequest{
		EntityID: request.EntityID, ExpectedRevision: request.ExpectedRevision,
		TraceID: request.Metadata.TraceID, RequestID: request.Metadata.RequestID,
	})
	if err != nil {
		return mutationFailure(MutationOperationDelete, err)
	}
	return MutationReceipt{KVRevision: response.ExpectedRevision}, nil
}

// ReadAuthoritative returns one validated entity and its same-entry revision.
//
// A read writes nothing, so every failure is not committed, with operation read-authoritative.
func (c *MutationClient) ReadAuthoritative(ctx context.Context, entityID string) (*graph.ExactEntity, error) {
	if c == nil || c.reader == nil {
		return nil, invalidMutationError(MutationOperationReadAuthoritative, errors.New("mutation client is nil"))
	}
	exact, err := c.reader.ReadExactEntity(ctx, entityID)
	if err != nil {
		// A read writes nothing, so every failure of it is not committed (#267): a classified
		// reply whatever its class, a timeout, a client that is not connected. The commit rule
		// for a mutation's error (ClassifyMutationError) is not asked.
		return nil, notCommittedError(MutationOperationReadAuthoritative, err)
	}
	return &graph.ExactEntity{Entity: exact.Entity.Clone(), KVRevision: exact.KVRevision}, nil
}

func (c *MutationClient) canonicalizeGroupMutation(
	operation MutationOperation,
	contractName string,
	groupName string,
	entityID string,
	input []message.Triple,
	metadata MutationMetadata,
	wantMode WriteMode,
) (contractBinding, PredicateGroup, []message.Triple, error) {
	binding, err := c.binding(operation, contractName)
	if err != nil {
		return contractBinding{}, PredicateGroup{}, nil, err
	}
	group, ok := binding.groups[groupName]
	if !ok || group.Mode != wantMode {
		return contractBinding{}, PredicateGroup{}, nil,
			invalidMutationError(operation, fmt.Errorf("contract %q group %q is not %s mode", contractName, groupName, wantMode))
	}
	matches, err := semtypes.MatchEntityIDPattern(binding.contract.EntityPattern, entityID)
	if err != nil || !matches {
		if err == nil {
			err = fmt.Errorf("entity %q is outside contract %q pattern", entityID, contractName)
		}
		return contractBinding{}, PredicateGroup{}, nil, invalidMutationError(operation, err)
	}
	triples, err := canonicalizeTriples(operation, input, metadata)
	if err != nil {
		return contractBinding{}, PredicateGroup{}, nil, err
	}
	allowed := make(map[string]struct{}, len(group.Predicates))
	for _, predicate := range group.Predicates {
		allowed[predicate] = struct{}{}
	}
	for index, triple := range triples {
		if triple.Subject != entityID {
			return contractBinding{}, PredicateGroup{}, nil,
				invalidMutationError(operation, fmt.Errorf("triple[%d] subject %q does not match entity %q", index, triple.Subject, entityID))
		}
		if _, ok := allowed[triple.Predicate]; !ok {
			return contractBinding{}, PredicateGroup{}, nil,
				invalidMutationError(operation, fmt.Errorf("predicate %q is outside group %q", triple.Predicate, groupName))
		}
	}
	return binding, group, triples, nil
}

func validateEntity(contract Contract, entity *graph.EntityState) error {
	if entity == nil {
		return errors.New("entity is required")
	}
	if err := semtypes.ValidateEntityID(entity.ID); err != nil {
		return err
	}
	matches, err := semtypes.MatchEntityIDPattern(contract.EntityPattern, entity.ID)
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("entity %q is outside contract %q pattern", entity.ID, contract.Name)
	}
	if !entity.MessageType.IsValid() {
		return errors.New("entity message type is required")
	}
	if contract.MessageType.IsValid() && !entity.MessageType.Equal(contract.MessageType) {
		return fmt.Errorf("entity message type %q does not match contract %q message type %q",
			entity.MessageType.Key(), contract.Name, contract.MessageType.Key())
	}
	return nil
}

func canonicalizeTriples(
	operation MutationOperation,
	input []message.Triple,
	metadata MutationMetadata,
) ([]message.Triple, error) {
	if operation == MutationOperationCreate || operation == MutationOperationAppend {
		if metadata.RequestID == "" {
			return nil, invalidMutationError(operation, errors.New("request ID is required"))
		}
	}
	// Every operation here requires Source; Reconcile also sends it as the
	// request's source, and graph-ingest replaces only that source's
	// statements of the group (design D15).
	if metadata.Source == "" {
		return nil, invalidMutationError(operation, errors.New("source is required"))
	}
	if operation == MutationOperationAppend && len(input) == 0 {
		return nil, invalidMutationError(operation, errors.New("at least one triple is required"))
	}

	triples := append([]message.Triple(nil), input...)
	for index := range triples {
		triple := &triples[index]
		if triple.Source != "" && triple.Source != metadata.Source {
			return nil, invalidMutationError(operation,
				fmt.Errorf("triple[%d] source conflicts with mutation metadata", index))
		}
		if triple.Context != "" && metadata.RequestID != "" && triple.Context != metadata.RequestID {
			return nil, invalidMutationError(operation,
				fmt.Errorf("triple[%d] context conflicts with mutation metadata", index))
		}
		if !triple.Timestamp.IsZero() && !metadata.Timestamp.IsZero() &&
			!triple.Timestamp.Equal(metadata.Timestamp) {
			return nil, invalidMutationError(operation,
				fmt.Errorf("triple[%d] timestamp conflicts with mutation metadata", index))
		}
		if triple.Source == "" {
			triple.Source = metadata.Source
		}
		if triple.Context == "" {
			triple.Context = metadata.RequestID
		}
		// The client never reads the clock: a statement's time is the
		// caller's, on the statement or in the metadata.
		if triple.Timestamp.IsZero() {
			if metadata.Timestamp.IsZero() {
				return nil, invalidMutationError(operation,
					fmt.Errorf("triple[%d] has no timestamp and the mutation metadata has none", index))
			}
			triple.Timestamp = metadata.Timestamp
		}
		if triple.ExpiresAt != nil {
			expiresAt := *triple.ExpiresAt
			triple.ExpiresAt = &expiresAt
		}
	}
	return triples, nil
}

func invalidMutationError(operation MutationOperation, err error) *MutationError {
	return &MutationError{
		Operation: operation, Kind: MutationInvalid, Class: errs.ErrorInvalid,
		Commit: CommitNotCommitted, Err: err,
	}
}

// mutationFailure reports a write's failure: a zero receipt, and the error carrying the commit state.
func mutationFailure(operation MutationOperation, err error) (MutationReceipt, error) {
	return MutationReceipt{}, ClassifyMutationError(operation, err)
}

// ClassifyMutationError maps the error of one graph mutation request to the commit state it
// proves, and returns nil for a nil error. It is the one mapping every MutationClient write uses.
//
// The error is not committed in two cases. Its chain holds a classified reply whose code
// graph-ingest sends only before it writes (invalid request, structural invalid, message type
// unregistered, entity ID invalid, entity ID authority invalid, revision mismatch, entity exists,
// entity not found, graph state reset required): the error keeps that reply's class, code and a
// copy of its detail. Or the request was not delivered, because no responder took it, natsclient
// refused it before sending (ErrNotConnected), or the caller's context ended
// before it was sent: Kind MutationUnavailable, class transient.
//
// Every other error, a classified reply with any other code included, is commit-unknown and fatal,
// so it is never retried: a write that may have committed could apply twice.
func ClassifyMutationError(operation MutationOperation, err error) *MutationError {
	if err == nil {
		return nil
	}
	if isDefiniteFailure(err) {
		return notCommittedError(operation, err)
	}
	return commitUnknownError(operation, err)
}

// notCommittedError reports a failure that wrote nothing. The class and code are a classified
// reply's; an unclassified request that was not delivered is transient, because a later attempt
// may find a responder (the raw nats.ErrNoResponders is unclassified, which errs.Classify reads as
// fatal); any other unclassified cause takes errs.Classify's class.
func notCommittedError(operation MutationOperation, err error) *MutationError {
	mapped := &MutationError{Operation: operation, Class: errs.Classify(err), Commit: CommitNotCommitted, Err: err}
	var classified *errs.ClassifiedError
	if errors.As(err, &classified) {
		mapped.Code = classified.Code
		mapped.Class = classified.Class
		mapped.Detail = maps.Clone(classified.Detail)
	} else if graphmutation.IsDefinitelyNotCommitted(err) || natsclient.IsNoResponders(err) {
		mapped.Class = errs.ErrorTransient
	}
	mapped.Kind = notCommittedKind(mapped.Code, mapped.Class)
	return mapped
}

// commitUnknownError reports a mutation that may have committed. It is fatal whatever class the
// reply carried: retrying a write that may have committed can apply it twice. The cause is held in
// a fatal *errs.ClassifiedError so errs.IsTransient, errs.Classify and errors.As read the same class
// as the Class field. The error keeps the reply's code and detail; the wrapper keeps the code, and
// the detail only when the code is not empty (errs.Classified).
func commitUnknownError(operation MutationOperation, err error) *MutationError {
	mapped := &MutationError{
		Operation: operation, Kind: MutationCommitUnknown, Class: errs.ErrorFatal, Commit: CommitUnknown,
		Err: errs.Classified(errs.ErrorFatal, err),
	}
	var classified *errs.ClassifiedError
	if errors.As(err, &classified) {
		mapped.Code = classified.Code
		mapped.Detail = maps.Clone(classified.Detail)
	}
	return mapped
}

// notCommittedKind names a not-committed failure by its reply code, and by its class when the code
// names no kind.
func notCommittedKind(code string, class errs.ErrorClass) MutationErrorKind {
	switch code {
	case graph.ErrorCodeInvalidRequest, graph.ErrorCodeStructuralInvalid:
		return MutationInvalid
	case graph.ErrorCodeEntityNotFound:
		return MutationNotFound
	case graph.ErrorCodeEntityExists:
		return MutationConflict
	case graph.ErrorCodeRevisionMismatch:
		return MutationRevisionConflict
	case graph.ErrorCodeGraphStateResetRequired, graph.ErrorCodeInternal:
		return MutationInternal
	}
	switch class {
	case errs.ErrorInvalid:
		return MutationInvalid
	case errs.ErrorFatal:
		return MutationInternal
	default:
		return MutationUnavailable
	}
}

// isDefiniteFailure reports a failure that proves the mutation did not commit: a transport
// outcome that proves no request was delivered, or a reply whose code graph-ingest sends only when
// it refused the request before its write: an invalid request, a revision conflict (an entity
// already present included, a create being a write on the condition that there is none), an
// entity not found, or a stored value the read before the write refused as graph state. Any other
// classified reply, whatever its class, can follow a write whose outcome the bucket did not
// confirm: natsclient's reply in place of a success response larger than the broker's payload
// limit is class invalid and follows the commit.
func isDefiniteFailure(err error) bool {
	var classified *errs.ClassifiedError
	if errors.As(err, &classified) {
		switch classified.Code {
		case graph.ErrorCodeInvalidRequest, graph.ErrorCodeStructuralInvalid, graph.ErrorCodeMessageTypeUnregistered,
			semtypes.ErrorCodeEntityIDInvalid, semtypes.ErrorCodeEntityIDAuthorityInvalid,
			graph.ErrorCodeRevisionMismatch, graph.ErrorCodeEntityExists, graph.ErrorCodeEntityNotFound,
			graph.ErrorCodeGraphStateResetRequired:
			return true
		}
		return false
	}
	return graphmutation.IsDefinitelyNotCommitted(err) || natsclient.IsNoResponders(err)
}

// classifiedAppendFailure treats a class it does not recognise as fatal, the
// same answer errs.Classify gives an unclassified error.
func classifiedAppendFailure(failure *graph.MutationFailure) error {
	class := errs.ErrorFatal
	switch failure.Class {
	case errs.ErrorInvalid.String():
		class = errs.ErrorInvalid
	case errs.ErrorTransient.String():
		class = errs.ErrorTransient
	}
	return errs.ClassifiedCode(class, failure.Code, fmt.Errorf("append failed: %s", failure.Code))
}
