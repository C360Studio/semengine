# ADR-110: The Write Seam Is the Pattern for Every `ENTITY_STATES` Writer

## Status

**Accepted (2026-10-10).** Ruled by the owner on #9 (comment 6097814764): every decision, and Q1, Q3 and Q4 as
recommended. Drafted on PR #164 (`Addresses #141`) under the owner's ruling of 2026-10-09 (epic #9 comment 6085243841,
"Ruled 2"): one of four records ruled before change 3's design is written, which cites all four. #141 stays open as the
adoption list.

Lines in this repository are cited on `main` at `9d1dd87` or on PR #93's branch at `3d66c91`
(`claude/setup-04a-02-ingest-kernel`, change 2, not yet merged), which establishes the seam; design D15, D21 and D23
and `entity_writes.go` were read at that head and are re-checked if they change before this record merges.
SemStreams is cited at the pin `8b99efe9` as `path:line`. Mechanics (error codes, counters, decoders, return values)
live in the `graph-entity-writes` and `transport-client` specs (change 2's deltas) and are cited here by requirement.

## Context

`ENTITY_STATES` holds each entity's current state: key the entity ID, value one `graph.EntityState` as JSON. A
*statement* is one `message.Triple` with its metadata (`Source`, `Timestamp`, `Confidence`, `Context`). A *revision*
is the number the bucket assigns to each committed write of a key; a *compare-and-set* writes only if the key is
still at the revision read. A *write seam* is the one place every change to a stored value passes through. A *lane*
is the path a change arrives on: the stream lane (a `Graphable` message), the mutation lane (a typed request on
`graph.mutation.>`), or the in-process lane (a call inside graph-ingest). A *write mode* is create, replace,
conditional replace, append or delete. *Poison* is a stored value or message the framework cannot read or must not
apply; it is refused or terminated, and counted.

**At the pin, one writer with seven hands.** graph-ingest is the only physical writer (`graph/kvcatalog.go:59`;
`processor/graph-ingest/component.go:404` is the only write port; ADR-091 §1), writing at seven sites on three lanes,
three of them read-modify-write closures of their own (`component.go:2104`, `:2460`, `:2655`; the single writes
`:2266`, `:2315`, `canonical_mutations.go:280`, `:348`), with three identity rules for one predicate
(`graph/helpers.go:98-108`; `message/triple_identity.go:62-71`; `canonical_mutations.go:630-640`), a private
`EntityState.Version` incremented at four sites (`graph/types.go:42-43`; `component.go:2164`, `:2493`, `:2684`;
`canonical_mutations.go:342`) beside the revision that is the fence (`graph/exact_entity.go:18`), no `Source` or
`Timestamp` stamped on the stream lane (`component.go:1813-1818`), a replace that ignores `Timestamp` (`:2154`), a
profile stamped from the clock (`:1891`), and a callback that gets only the bytes (`natsclient/kv.go:324` on `main`).

**What the pin's records decided.** ADR-049 (the lifecycle manager emits and never writes, `:136-139`;
`ExpectedRevision` is its compare-and-set-on-condition, `:199-228`), ADR-054 (the indexing profile is single-valued,
set at birth, replace-on-write, `:123`, `:216`), ADR-055 (four lanes; the stream lane appends, `:127-128`, `:376`; no
birth without a *semantic envelope*, the registered message type and indexing profile a birth carries, §2; append is
must-exist, §3), ADR-072 (the stream replace is arrival-order newer-wins regardless of timestamp, `:96-99`, `:200`),
ADR-073 (Proposed, design-only: no TTL or size cap on the bucket), ADR-074 (one authoritative codec gates every write;
persistence infers no authority from `Source`, `:50-54`; rule `update_kv` cannot write framework buckets, `:64`),
ADR-079 (byte-authoritative enforcement; arrivals blocked by resident poison are Nak'd, not terminated, `:50`; repair
is delete then create, §5), ADR-090 (canonical current state; no event sourcing) and ADR-091 (sole physical writer;
four typed operations; atomic create for birth and observed-revision compare-and-set for every other write, the
hierarchy inverse included, `:63`, `:84`; a lost reply is *commit-unknown*: the request may have been applied).

**The evidence.** #100, #98, #99, #145, #147, #148 and #150 were each argued inside PR #93, which answered them with
one seam (`processor/graph-ingest/entity_writes.go`, held by `TestEntityWritesHaveOneSeam`), one primitive
(`UpdateWithRetryRead`, D23, accepted #91 comment 6066791396) and the `graph-entity-writes` delta. Clustering,
embedding, spatial, temporal and the anomaly store each carry a compare-and-set of their own.

## Decision

Each decision is numbered so it can be ruled on alone. "Keeps" and "changes" name the pin's record and the ruling.

### Decision 1. One physical writer, one seam; everyone else is a caller

graph-ingest is the only code that writes an `ENTITY_STATES` value (keeps ADR-091 §1, ADR-049 `:136-139`, ADR-079 §2).
Inside graph-ingest every write is in one file, one method per mode, held by a package test (spec "One rule per write
mode", last sentence). Outside it the catalog declares the rule and does not enforce it: the descriptor is `Write:
natsclient.WriteOwnerOnly` (`graph/kvcatalog/kvcatalog.go:51` on #93; `natsclient/kvspec.go:52-54`), readers get a
read-only `CatalogReader` (`:248`, `:270`), and `IsFrameworkOwnedBucket` (`:214-216`) is read by nothing but tests. The
one refusal is the pin's: a rule `update_kv` naming such a bucket is refused at load, execution and acquisition
(`processor/rule/config_validation.go:382`, `actions.go:2264`, `kv_writer.go:87`; ADR-074 `:64`), and it returns with
the rule processor in change 6. A non-owner calling `EnsureCatalogBucket` (`:235-243`, a write-capable handle behind a
comment) or opening a raw write handle is review only. Everything else changes entity state through the four typed
operations (keeps ADR-091 §2). `pkg/lifecycle` is a caller: its compare-and-set is the seam's conditional replace and
delete at a revision from an exact read, re-read on mismatch for a create or a transition and returned on mismatch for a
despawn (`pkg/lifecycle/manager.go:433-450`, `:560`, `:742`, `:882-890` on #93), with a reader (`:194`) and no write
handle (keeps ADR-049 `:136-139`, ADR-091 §1; not open).

### Decision 2. One primitive for a read-modify-write

A read-modify-write on a bucket the framework owns uses `natsclient.KVStore.UpdateWithRetryRead` (`natsclient/kv.go
:344` on #93): the callback gets the bytes and the revision it read, declines with `ErrKVSkipWrite`, and the call's
result is only its own commit (spec `transport-client`, "A retried update passes the revision it read"; D23). There is
one retry loop for graph-ingest, natsclient's (#91 comment 6065395072 rejected a second loop inside graph-ingest);
callers of the typed operations keep a bounded re-read and retry (`pkg/lifecycle/manager.go:508`, `:560`, `:742`). A
birth is the bucket's atomic `Create`; a caller-fenced write is `Update` or `DeleteAtRevision` (keeps ADR-091 §3 for
the lanes that remain; ruling M, #145, removes the hierarchy-inverse write ADR-091 `:63` and `:84` name). The seam
declines three cases: an append that adds nothing, a stream arrival on a profiled entity whose every set is older or
that carries only the profile, and an unchanged reconcile (spec "One rule per write mode", "Timestamp orders a
replace"). Rejected (D23): declining by returning nil (an empty value is storable, and is poison) and a `written`
flag beside a revision the call did not commit. Cost: 0 attributes no commit to the call, not "nothing committed".

### Decision 3. The revision is the only fence, and there is one stored revision

A stored entity carries no version of its own (`EntityState.Version` dropped; #100; changes the pin's
`graph/types.go:42-43`); the revision is the only fence and the only revision a reply reports (keeps ADR-091 §3-4;
spec "The revision is the only fence"). `ENTITY_STATES` keeps `History` 1 (#99, ruled; keeps the pin's decision at
`graph/kvcatalog.go:61-68`): no point-in-time read; history is the input stream (keeps ADR-090). Binds `ENTITY_STATES`
only; re-decided only with a design for a named consumer need.

### Decision 4. One identity rule per write mode, the same on every lane

| Mode | Lanes | Identity of a stored statement | Fence |
| --- | --- | --- | --- |
| create | mutation create; in-process create; the stream lane when the key is absent | none; an existing key is refused | the bucket's `Create` |
| replace | the stream lane | (subject, predicate, source): each source replaces its own set of a predicate whole; other sources' statements stay | the revision read; `Timestamp` orders it |
| conditional replace | mutation reconcile | as replace; the request names one source and carries only its statements | the caller's expected revision |
| append | mutation append | `message.AppendIdentityKey` (subject, predicate, datatype, source, context, object); must-exist | the revision read |
| delete | mutation delete | none | the caller's expected revision |

Replace keyed on (subject, predicate, source) is ruling A (#91 comment 6037287957); it **changes** ADR-072's
arrival-order replace per (subject, predicate) (`:96-99`, `:200`), which had replaced ADR-055's stream append
(`:127-128`, `:376`). Must-exist append keeps ADR-055 §3; a reconcile names one source (ruling 1, #91 comment
6037604840). "Unchanged" is an equality of values, not an identity. A predicate may hold statements from several
sources, so the single-value reads pick the latest `Timestamp`, then the source that sorts first, then stored order
(ruling 2; changes the pin's first-match, `graph/types.go:49-61`). A producer's indexing profile sits beside
graph-ingest's (ruling 4; changes ADR-054's replace-on-write, `:216`).

### Decision 5. Statement metadata is required, and `Timestamp` orders a replace

Every stored statement has a `Source` and a `Timestamp`, and graph-ingest defaults neither from the clock; the stream
lane stamps from the envelope or refuses as poison; derived statements carry the write's latest statement time; an
originator may assert its own (#98, ruled, #91 comment 6036308354; #91 comment 6066791396; spec "Statement metadata
is required"; changes the pin's `component.go:1813-1818`, `:1891`). On the stream lane a set older than the stored
set of its (predicate, source) is not applied and the result says so; a conditional replace is fenced by the caller's
revision, not ordered (ruling C; spec "Timestamp orders a replace"; changes ADR-072's "regardless of timestamp",
`:96-99`). `Confidence` and `Context` never order. The stream lane refuses the reserved sources graph-ingest and
`pkg/lifecycle` write under (ruling 5; `graph.IsReservedSource`), which narrows ADR-074's "no authority from
`Source`" (`:50-54`); the mutation lane does not refuse them (D15, `design.md:746` on #93). No record owns the
principal-bearing envelope ADR-074 defers; the gap is declared, and where it closes is Q3.

### Decision 6. Fail closed on a value the seam cannot change; a delete is not refused

A stored value the seam cannot change, read with the *trusted decoder* (the owner's own decode, without the full
predicate contract) plus the key check, is refused with nothing written and the refusal recorded at the revision read
(spec "The write path refuses a stored value it cannot change"; keeps ADR-079 §1 and ADR-074's one codec, which still
gates every commit). The stream lane leaves a refused arrival unacknowledged, bounded (keeps ADR-079 `:50`); the one
case D23 accepted **changes** it: an arrival whose every set is older over such a value is acknowledged with nothing
written. A delete removes the entity at the caller's revision whatever the value (ruling O, #148), which lets ADR-079
§5's repair run; auto-delete stays rejected. A birth on a lane that infers hierarchy fails closed (D21; ruling B for
the mutation lane; ruling M: no inverse edge).

### Decision 7. Where the pattern stops

Decision 2's first sentence binds every read-modify-write over a bucket the framework owns, a catalog row with
`WriteOwnerOnly` (`IsFrameworkOwnedBucket`), at that package's port and never as a migration obligation (#141): the
pin's `graph/clustering/summary_store.go:179`, `graph/embedding/storage.go:308`, `:367`, `:438`,
`graph/inference/storage.go:159` (`ANOMALY_INDEX`), `processor/graph-index-spatial/component.go:944`,
`processor/graph-index-temporal/component.go:1054`, `:1138`; what each loop decides stays its own (ADR-087 for the
summary store). Whether that binding is ruled is Q1. Decisions 3 to 6 bind the one `ENTITY_STATES` writer. Each lane
or caller keeps its validation and authority before I/O, replies, metrics, acknowledgement order and redelivery guard
(ADR-072), poison bookkeeping, and what a birth carries (ADR-054, ADR-055 §2). Not decided here: how a *follower* (a
component that watches the bucket and derives a view) watches and records a sticky reset state (ADR-108, #109); what a
committed revision means to readiness (ADR-066; ADR-109, #110); the *effect fence*, a durable record that an external
side effect was attempted so a replay does not repeat it, with the journal and terminal ownership (ADR-111, #24).
`AGENT_LOOPS` is not a catalog bucket and is out of scope.

### Open for the owner

**Ruled 2026-10-10 (owner, #9 comment 6097814764): Q1, Q3 and Q4 as recommended.** The questions and their costs stay
below as the record of what was weighed.

Q2 of the inventory, whether `pkg/lifecycle` is a writer, is settled in Decision 1 (keeps ADR-049 and
ADR-091 §1) and is not open; the numbering below keeps the inventory's.

- **Q1 (Decision 7).** Does Decision 2 bind every read-modify-write over a framework-owned bucket at its port?
  Recommended: yes. Its cost: spatial, temporal and the anomaly store gain a retry they do not have (the anomaly
  store's read-then-`Put` at revision 0, `storage.go:146-166`, loses its lost-update hole); embedding's and
  clustering's guards move inside the callback. The other answer carries four hand-rolled loops and four one-shot
  revision checks in three stores into changes 4, 5 and 7.
- **Q3 (Decision 5).** Where does the reserved-source gap on the mutation lane close? Recommended: declare it here
  and name epic #84 (security primitives: authn, authz) as the likely home of ADR-074's principal-bearing envelope.
  Refusing now needs a wire identity `pkg/lifecycle` does not have.
- **Q4 (Decision 2).** A stream re-arrival equal to the stored set is applied (`graph/helpers.go:48`,
  `entity_writes.go:199-200` on #93) and rewrites `UpdatedAt` (`:225`), so the revision rises and every follower
  wakes. Recommended: declare it as a cost; re-decide with the first reader that pays for it (change 4). The other
  answer reopens frozen #93 and #98's "equal timestamps apply in arrival order".

## Alternatives rejected

Port the lanes as they are, or write each lane's rule into the spec (#100; D15 a and b). A replace keyed on
(subject, predicate): the cross-source wipe (D15 c; ruling A). A second retry loop in graph-ingest (#91 comment
6065395072). `History` above 1 for a consumer no one has named (#99). Refusing reserved sources on the mutation lane
now (Q3).

## Consequences

- Each port that writes entity state, and change 3's design, cites this record; a port that keeps a hand-rolled
  compare-and-set says why against Decision 2. A consumer's bill: `Source` and `Timestamp` on every statement (the
  typed client refuses before sending); an exact read before a conditional write; commit-unknown on a lost reply.
- Readers inherit the single-value pick (changes 4, 6 and 7). Three declared costs: a no-op over a value only the
  full contract refuses reports unchanged; an all-older arrival over such a value is acknowledged; an equal
  re-arrival rewrites (Q4).
- `natsclient`'s ledger row carries three `adapt` items (the callback reads the revision; a skip passes through
  `UpdateWithRetryRev`, `UpdateWithRetry` and `UpdateJSON` as no error; commit attribution;
  `docs/admission-ledger.yaml:1599-1605` on #93).
- A watcher sees one revision per applied write and none for a declined one; that unit is available to ADR-109.

## Related

ADR-049, ADR-054, ADR-055, ADR-072, ADR-073, ADR-074, ADR-079, ADR-090, ADR-091 (SemStreams at `8b99efe9`; ADR-056
superseded by ADR-091; ADR-066 and ADR-087 named for their boundaries) · ADR-108 (#109), ADR-109 (#110), ADR-111
(#24) · D15, D21, D23 and the `graph-entity-writes` and `transport-client` deltas of `setup-04a-02-ingest-kernel`
(PR #93) · #100, #98, #99, #141, #145, #147, #148, #150, #84 · #91 comments 6037287957, 6037604840, 6065395072,
6066791396, 6080973822 · #9 comment 6085243841
