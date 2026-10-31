# ADR-085: Gate on Health, Report Freshness

> **Ported from SemStreams.** This record is `docs/adr/085-gate-on-health-report-freshness.md` at SemStreams commit
> `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. It was carried without code for SemEngine issue #157 (owner ruling,
> comment 6085722578), with ADR-066, ADR-083, ADR-084 and ADR-088, so that SemEngine's readiness ADR (ADR-109, issue
> #110) can cite it. Its admission-ledger row is `docs/adr/085-gate-on-health-report-freshness.md` in
> `docs/admission-ledger.yaml`. The decision text is unchanged apart from a comment that keeps markdownlint from
> flagging decision 7, a list that starts at 7. Read it with these facts about this repository:
>
> - Issue numbers in the text below (`gh#590`, `gh#605`) are SemStreams' or a sister repository's, not SemEngine's; the
>   OpenSpec change it names, `fusion-consistency-simplification`, is SemStreams'.
> - Of the records named here, ADR-066, ADR-083 and ADR-084 are ported with this one. ADR-081 is ported with
>   `pkg/graphview` (change `setup-04a-04-graph-roots-graphview`, draft PR #476); ADR-082 is not yet ported.
> - Decision 5 is kept: `pkg/graphview` has `Applied()` and the snapshot pair `Snapshot.Revision` and
>   `Snapshot.AppliedAt`, and no `graphview` API gates on them. The package changed two things Decision 5 does not
>   mention: `Start` and `Stop` became `Run(ctx)`, and ending `Run`'s context is a stop, not a watcher loss.
> - Decision 7 is kept in `graph/clustering` (change `setup-04a-04-graph-roots-clustering`, draft PR #483):
>   `Maintainer.Tick` writes every changed group record first and removes records (groups that became empty, and on
>   the first tick the keys of groups that do not exist) only after every write succeeded, so a reader of the bucket
>   sees old records, never an empty index. One difference: a failed removal makes `Tick` return an error and the
>   removal stays pending for the next `Tick`; it does not end the run with the superset standing as the pin's
>   prune did. The cache keyed by `(level, ID)` is `processor/graph-query`'s and is not ported yet. The bucket key
>   `{level}.{prefix}` already carries the level.
> - Apart from `pkg/graphview` and `graph/clustering`, none of the code named here is ported: `processor/graph-clustering`,
>   `processor/graph-index`, `processor/graph-query`, `graph/query`'s client (retired before the pin), `pkg/fusion`,
>   and the types and functions named with them (`EvaluateReadinessGate`, `IndexStatusResponse`,
>   `readiness.MinBoundedStaleness`, `LPADetector`, `CommunityCache`).

## Status

Accepted — 2026-07-21. Decision record for the second half of the
`fusion-consistency-simplification` change. **Supersedes ADR-084 D1's freshness
parameter and D5's "bounded-staleness becomes the freshness parameter" clause**,
and **completes the retirement of ADR-082** — 082's consumer-class split was
already superseded by ADR-084; its bounded-staleness *tolerance* is now deleted
outright rather than reparameterized. ADR-066, ADR-081, and ADR-083 stand;
ADR-083 D3 (staleness measured in time, from KV commit timestamps) is retained
in full and is in fact the thing this ADR makes load-bearing.

## Context

ADR-084 retired the absence license: coverage cannot answer whether a source ever
published, so no signal in the system licenses "not returned ⟹ not in the graph."
That was correct and it landed. **It also half-finished the job.**

Coverage-derived *gating* existed for exactly one reason — to serve that license.
"Empty means not-found only if `Ready`" was the argument for making consumers wait
until the index caught up. ADR-084 removed the argument and freed the read paths,
but left the machinery standing for everyone else: a public `Freshness` type, three
constructors, a satisfiability floor, two defer reasons, and an operator knob.

The residue does not survive inspection:

- **One call site.** `FreshnessWithin` is constructed in exactly one place,
  `processor/graph-clustering/component.go`. Every other consumer asks
  `FreshnessNone` (`graph/query/client.go`, `pkg/fusion/engine_lens.go`) or
  `FreshnessExact` (`processor/graph-index/query.go`) — and that last one is a
  bootstrap probe wearing a freshness costume: its own comment concedes the
  staleness comparison is meant to be unreachable there. A survey of all ~20 local
  `sem*` repos found **zero** adopters of `max_staleness` or its predecessor
  `index_lag_tolerance`.

- **The knob needed a floor derived from an unrelated constant.** `max_staleness`
  had to exceed the publish heartbeat, because the judged age includes envelope
  arrival age and arrival age sweeps a full heartbeat. A tolerance whose valid
  range is set by the transport's tick rate is exposing plumbing as policy. The
  general form: *you cannot bound a quantity below the interval at which you learn
  it* — and a faster probe does not rescue it, since KV `Get` and `Watch` are
  equally stale on a tick-published key.

- **The one consumer's real requirement was "no gate."** semboids recorded 0
  detections at the default and 6 at `max_staleness: "3s"`; we later measured that
  3s is satisfiable on only ~52% of the heartbeat window, so those 6 were a
  heartbeat accident, not a tuned value. What community detection actually wants is
  to run periodically on whatever the graph is now. It is the safest possible
  consumer of a stale view — periodic, idempotent, self-correcting — and a
  partition missing 200 of 500k edges is not detectably wrong.

  **A first draft of this ADR said "its partition overwritten next cycle." That
  was false, and review caught it.** `LPADetector.DetectCommunities` cleared
  COMMUNITY_INDEX *before* rebuilding, so the swap was delete-then-write, not
  overwrite. Ungating detection would have turned a rarely-reached window into a
  near-permanent one — with runs measured at up to 23.7s inside a 30s interval,
  the index would have been empty most of the time, and graph-query's cache
  latches ready-once so it would have served that emptiness as an answer. The
  claim is now true because decision 7 made it true, not because it was checked.

Meanwhile `pkg/graphview` (ADR-081) had independently arrived at the right shape
and demonstrated the asymmetry. Its only gates are `ErrNotReady` until the initial
WatchAll replay completes and a fail-closed path on watcher loss. It has no age
gate — and no age *report* either, exposing `AppliedRevision()` with no time
dimension. Two packages solving one problem, each holding the half the other was
missing.

## Decision

1. **A readiness gate asks exactly one question: is this index sound to read
   from?** Four conditions, none of them optional and none consumer-specific:
   the status reading is fresh (the producer is talking to us *now*); its `State`
   is one we recognize (allow-list, never a deny-list); it is not a hard stop
   (`degraded`, `reset_required`); and `bootstrap_complete` is set. Anything else
   proceeds. `EvaluateReadinessGate` takes a reading and returns a verdict — there
   is no second parameter, because there is no second question.

2. **Staleness is reported, never gating.** `IndexStatusResponse.StalenessMs`
   survives intact, including its presence encoding. What changes is that nothing
   consults it to decide whether to answer. A consumer running on a stale view
   stamps the age it ran at onto its own output — community detection records
   `staleness_at_detection_ms` on every *verified* run rather than only the runs a
   tolerance admitted. The number moves from the admission decision to the result.

3. **`Freshness`, `max_staleness`, and their supporting machinery are deleted**,
   not deprecated: the `Freshness` type and its three constructors, the
   `over_staleness` and `staleness_unknown` defer reasons, the bound arithmetic,
   and `readiness.MinBoundedStaleness` / `ValidateStalenessBound`. A config still
   carrying `max_staleness` fails startup loudly with a message naming what
   replaced it — a silently-ignored knob is worse than a removed one.

4. **The surviving defer reasons are four, and each names a distinct operator
   action**: `status_unknown` (the feed is dead or the producer is absent),
   `unrecognized_state` (version skew — the producer is talking, and saying
   something we do not understand), `hard_stop` (broken; operator intervention),
   `bootstrap_incomplete` (still building; wait). None of them is answered by
   tuning a tolerance, which is precisely why no tolerance exists.

5. **`pkg/graphview` gains the reporting half and keeps gating unchanged.** It
   tracks the KV server write time of the newest applied revision alongside the
   applied revision watermark and exposes them as one atomic pair, carried on
   snapshots as well. No graphview API gates on it. The package's existing
   bootstrap and fail-closed gates are already exactly what decision 1 prescribes
   and are not touched.

6. **What readiness honestly indicates, stated so it can be checked.** Provable
   and therefore gateable: the producer is reachable now; it is emitting a state we
   understand; it declares itself broken; it finished its initial replay. Not
   provable and therefore not gateable: that the view is within N ms of the writer
   (`staleness_ms` is a *floor* — an undelivered revision cannot age it, it crosses
   a server clock against a local one, and it is observed at heartbeat
   granularity); that an empty result is an absence (ADR-084); that any multi-read
   answer was a consistent cut (why `Coherent` was deleted in the first half of
   this change). Consumers needing a real snapshot use graphview, which has one.

The framing that survived every restating of this argument, and which the docs
should lead with: *the index is a materialized view. It is either still building,
broken, or working-and-N-seconds-behind. Readiness answers which. Only the first
two are gates.*

<!-- markdownlint-disable-next-line MD029 -->
7. **Ungating a periodic rebuild requires that rebuild to be non-destructive —
   at every layer that observes it.** Community detection replaced its partition
   by clearing COMMUNITY_INDEX and then rebuilding into it. That is tolerable at a
   rate of approximately never and indefensible every tick, so detection now writes
   the new partition over the prior one in place and, at the end of the run, lists
   the current key set and deletes everything outside the new partition. A reader
   of the *bucket* during a rebuild sees the union of old and new — stale entries,
   never an empty index. A failed prune leaves a correct superset rather than
   failing the run.

   **The union guarantee holds at the bucket and had to be earned at the cache.**
   Moving the deletes after the writes broke `processor/graph-query`'s
   `CommunityCache`, which keyed communities by bare ID while storage keys by
   `{level}.{id}`. Because community IDs are seed entity IDs drawn from one pool at
   every level, a late delete rebuilt a level index *after* higher-level writes had
   shadowed it, collapsing the level-0 index that GlobalSearch reads with no
   fallback. Under the old delete-first ordering that rebuild happened before the
   shadowing, so the defect was latent. The cache is keyed by `(level, ID)` as part
   of this change; without that, this decision trades an empty window for a
   truncated one and is not obviously an improvement.

   This is the same principle as decision 2 one layer down: the honest degraded
   state of a materialized view is *stale*, not *absent*. Two things generalize:
   **removing a gate makes the gated work's failure modes load-bearing, and a
   destructive rebuild that was previously unreachable becomes a permanent
   window**; and **a non-destructive rebuild is only non-destructive if every
   downstream projection agrees with the store about what identifies a record.**
   Anything else we ungate needs both questions asked of it first.

## What "done bootstrapping" means, precisely

Because decision 1 makes `bootstrap_complete` the only coverage-shaped condition
left in the gate, its definition is now load-bearing and belongs here rather than
in a comment.

It is **per-producer and per-process**, latched, never cleared. graph-index sets it
when initial enumeration has delivered every key that existed at attach time *and*
the applied floor has reached the revision enumeration ended at — a **fixed target
snapshotted at enumeration end**, not the live stream head. `Ready` also latches it
as a sufficient condition (catching up to the live target implies catching up to
any earlier one). graph-embedding uses the same shape but requires each enumerated
entity to reach a terminal embedding outcome, and deliberately has no `Ready`
shortcut, because delivery is not application.

Two properties are worth stating because both have been gotten wrong:

- It is **not** a claim about graph contents. The authoritatively-empty graph
  latches complete and serves.
- It is **not** "the index reports ready." `Ready` is coverage against the *live*
  head, which under continuous write is a measure-zero instant — gh#590's original
  bug, and a latch against a moving target would never flip under a firehose.

## Alternatives rejected

1. **Keep the knob with a better-derived floor.** Rejected: the floor is the
   symptom. A tolerance that must be validated against the publish heartbeat is
   reporting that the system cannot answer the question the tolerance asks.

2. **Keep the parameter but default every consumer to "none."** Rejected: it
   preserves a public type, three constructors, and two defer reasons against a
   hypothetical future consumer, and leaves the gate able to defer for a reason no
   operator can act on. If such a consumer appears, it will want to *stamp* the age
   on its output — which decision 2 already provides — and the case for reviving
   admission control should be made on that consumer's evidence, not pre-built.

3. **Keep `FreshnessExact` for graph-index's own responder.** Rejected: it is a
   bootstrap probe. `bootstrap_complete` answers it directly and more honestly, and
   the exactness there was demonstrably producing one spurious transient
   `IndexNotReady` at the moment the latch flipped under continuous write —
   reachable by tracing `latchBootstrap`, though never measured in the field.

4. **Gate graphview on view age for symmetry.** Rejected, emphatically — the
   symmetry runs the other way. graphview was right; the rest of the system is
   being brought to it.

## Consequences

Community detection runs whenever the index is healthy, at whatever age the view
happens to be, and reports that age on every run. gh#590's symptom cannot recur in
any tuning, because there is no tuning. The tuning-dynamics problem filed as
gh#605 dissolves rather than being solved — detection duration can no longer
interact with a tolerance it must fit inside.

This is a breaking config change riding the same wave as ADR-083's and ADR-084's,
so sister repos migrate the readiness surface once. Its blast radius is smaller
than it looks: `max_staleness` has no adopters anywhere, so the break is
theoretical for every consumer we can see, and the loud startup rejection exists
for the one we cannot.

The system loses the ability to withhold an answer because a view is behind. That
capability was exercised once, by a consumer that did not want it, in service of a
license that no longer exists. What replaces it is an answer with its age attached
— which is strictly more information than a refusal carried.

The honest residue: a consumer that genuinely cannot act on stale data now has no
framework support for saying so, and would have to compare `staleness_ms` itself.
That is the correct place for the decision to live until a second such consumer
proves the abstraction, and it is a deliberate reversal of the instinct that
produced `Freshness` in the first place.

## SemEngine disposition (ADR-109)

ADR-109 (`docs/adr/109-ready-after-boot.md`, its table "The pin's records: kept, changed or dropped") keeps D2
(staleness reported, never gating) and the fixed-target definition of "done bootstrapping" (:167-189) as what the
responder checks in process. It changes D1: the gate is not a cross-process contract; graph-index asks its conditions
in process, and nothing asks them across a process. D4's defer reasons ship on PR #93 and are logged by that reader
only.
