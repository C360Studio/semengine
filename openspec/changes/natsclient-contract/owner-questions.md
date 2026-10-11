# Owner questions: natsclient's contract (PR #177, #170)

Words used below:

- A **probe** is a small program or test, run against the exact nats.go and nats-server versions this repository
  uses, with its output kept.
- A call is **held** when the NATS server stops reading the connection and the send buffer is full. Every write
  then waits behind the stuck one, until the server reads again or nats.go's one-minute write timeout ends that
  write, and the next write can stick the same way, so no upper limit was seen.

## 1. Ship the record as documents only; fix the code comments later under #168

**Recommendation: yes.**

- A documents-only pull request needs no Codex review, so the record that gates change 3 lands sooner.
- The wrong comments are 16 places in natsclient's Go comments and README (design D1). None is in a part of a file
  that #93 changes.

**Cost of fixing them here:** this becomes a code pull request and waits for Codex's review. You get one pull request
instead of two.

## 2. Accept that natsclient does not bound a held call; #168 becomes a docs fix

**Recommendation: yes.**

- The wait comes from nats.go, which uses one lock per connection. Only the server reading again, or the write
  timeout, releases the stuck write.
- Each way to cut the wait short adds natsclient machinery, which you ruled out (ADR-112, "Alternatives rejected").

**Cost of saying yes:** a shutdown can be held for minutes if the server stops reading (3 m 1.6 s in one run; still
held at 3 m 20 s, when the probe stopped, in two others), and change 3's design must say so.

**Cost of the other answer:** new natsclient code. The simplest option, a shorter write timeout, can drop publishes
on a slow link. That last point comes from reading nats.go's code; it was not measured.

## 3. A test in the repository counts as a probe

**Recommendation: yes.**

- A test that runs the real nats.go re-runs with every `task verify`, so it stays checked after an upgrade.
- A test against a stand-in object instead of nats.go does not count.

**Cost of the other answer:** the 28 statements that tests back today would each need a separate program, posted and
re-run by hand.

## 4. The rule covers only text a change writes or edits

**Recommendation: yes.**

- It does not reopen every inherited sentence, and it agrees with `.agents/protocol.md:47-49` ("no pin experiment
  per finding"). The pin is the SemStreams commit the code was copied from.
- Writing "not promised" is always allowed in place of a probe.

All 16 statements on `main` shown false get fixed: 5 in this pull request and 11 under #168. The 30 unmeasured
statements stay until a change touches them.

**Cost of the other answer:** each of those 30 needs a probe or a "not promised" before anything else lands.

## 5. Rewrite #168's description; leave the archived #156 design as written

**Recommendation: yes. The coordinating agent edits #168 after you accept.**

- #168's reading, "returned 59.7 s after the server read again", was mistimed: the test program's own status check
  waited on the same lock. Measured properly, a call returns within 29 ms of the server reading again.
- #168 is live, and readers will act on it. The archived design records what was decided then, and ADR-112 carries
  the correction.

**Cost of also editing the archive:** it changes the record of a past decision, and adds one more file here.

## 6. Leave transport claims in other records to the changes that rely on them

**Recommendation: yes, with a one-line note on #110.**

- ADR-110 says a watcher sees every revision. SemStreams' ADR-066 says a watcher that falls behind does not.
  Readiness (#110, ADR-109) is the work that needs the answer.
- Redelivery after a process kill is already #93's task 4.4.

**Gap left:** redelivery on a file-backed stream after the NATS server restarts is tested nowhere. ADR-112 lists it
as not promised.

**Cost of the other answer:** new probes, which this pass dropped.

## 7. Before merging a nats.go or nats-server upgrade, re-run the quick probes

**Recommendation: yes.**

- The approved plan says "Upgrades need relevant regression evidence" (`docs/setup-plan.md:235-236`). #83 already
  asks for a re-check on each nats.go upgrade.
- ADR-112's lines on held calls, `Connect`, `Publish`, KV writes and reconnect rest on posted programs, which
  `task verify` does not run.
- P10, P12, P13, P15 and p15b take about a minute. P11 and P14 take several minutes, and run only if P13's result
  changes.

**Cost of the other answer:** after #118 (nats-server 2.15.0) merges, those lines describe 2.14.7, and nothing
flags it.

## 8. No rule text in the developer and technical-writer contracts

**Recommendation: yes.**

- The reviewer contract's new bullet applies to every change, including the comments and READMEs those roles
  write.
- It avoids a fourth place to keep in step.

**Cost of the other answer:** two more contract edits. The rule would then be seen when the text is written, not
first at review.

## 9. Leave the two error checks that match on error text as they are

**Recommendation: yes, recorded in ADR-112 as "not promised".**

- Ruling N named three other functions.
- These two (`isAlreadyExistsError` and `isCircuitNeutralStreamCapacityError`) read nats-server's wording. That is
  why ADR-112 does not promise their answers.

**Cost of the other answer:** a small code issue now, needing Codex's review.

## 10. Cite SemStreams' ADR-060; do not copy it in

**Recommendation: yes.**

- ADR-060, the error format on the wire that `natsclient/errors.go` follows, is background here.
- Copying an ADR in is the separate job #176 does, with an admission-ledger row and its own review.

**Cost of the other answer:** a copied ADR plus a ledger row. The ledger is YAML, so this would become a code pull
request.

SemStreams' ADR-077 is not copied either; nothing here relies on it.
