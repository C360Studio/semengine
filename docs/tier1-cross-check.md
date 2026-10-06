# Tier-1 cross-check of the tier-0 port set

This is a record, not a rule. It compares SemEngine's tier-0 port set with SemStreams' Tier 1 list and changes no
admission-ledger row. Task 5.2 of `setup-04a-01-floor` records it, carrying #9 "Carried to 04A" item 1 and
SETUP 03B task 2.2.

Two terms first:

- **Tier 1** here means SemStreams' *package* tier: the 62 packages listed in `release/tier1-packages.txt` at the
  SemStreams pin `8b99efe9`. These are the packages a sister repository imported on 2026-09-02, frozen under
  SemStreams' ADR-106. It is not a SemEngine capability level. SETUP 03B ruled (Q2, #2) that SemEngine makes no
  compatibility promise to it.
- **The ruled 65** is the tier-0 port set: 65 packages, 140,842 non-test lines (SETUP 03B design D4, owner ruling
  #8 comment 5932313950). It is the first-pass `go list` closure of 65, less the ten packages the rulings cut, plus
  the ten they admitted.

The setup plan asks for this cross-check when the ledger is seeded (`docs/setup-plan.md`, "seed the ledger from the
measured closure and cross-check it against SemStreams' sister-import list"). It was first measured on the
first-pass 65 (SETUP 03B `inventory.md` A7: 39 / 23 / 26). This page re-measures it on the ruled 65.

## How it was measured

Read-only, on 2026-10-04. The SemStreams pin came from `gh api repos/C360Studio/semstreams/tarball/8b99efe9c66a4faa4fa509f9f62cc6bad8392128`,
unpacked to a scratch directory; no sister checkout was used. In the unpacked pin:

```bash
# 1. The first-pass 65 (03B inventory A1, same 28 roots)
GOFLAGS=-mod=mod GOWORK=off go list -deps ./component ./config ./gateway/graph-gateway ./graph ./message \
  ./metric ./model ./natsclient ./payloadregistry ./pkg/buffer ./pkg/errs ./pkg/fusion ./pkg/fusion/fusionnats \
  ./pkg/fusion/fusionvocab ./pkg/projection ./pkg/retry ./pkg/types ./processor/graph-embedding \
  ./processor/graph-index ./processor/graph-ingest ./processor/graph-query ./service ./storage \
  ./storage/objectstore ./storage/storeregistry ./types ./vocabulary ./vocabulary/cco \
  | grep 'c360studio/semstreams' | sort -u > first65.txt                        # → 65 lines

# 2. The ruled 65 = first65 − the ten cut + the ten admitted (03B design D4)
#    cut:      agentic gateway/graph-gateway vocabulary/agentic agentic/agentrun model/wire graph/llm gateway
#              internal/deliverylane internal/agentterminal internal/looptoken
#    admitted: processor/rule processor/rule/expression processor/graph-clustering output/websocket
#              processor/graph-index-temporal processor/graph-index-spatial vocabulary/export graph/geo/geojson
#              internal/maxdelivery composition/cli
#    (each cut package is in first65.txt; no admitted one is)                     # → ruled65.txt, 65 lines

# 3. Tier 1
grep -v '^#' release/tier1-packages.txt | grep . | sort -u > tier1.txt          # → 62 lines

# 4. The cross-check
comm -12 tier1.txt ruled65.txt | wc -l   # both            → 39
comm -23 tier1.txt ruled65.txt | wc -l   # Tier 1 only     → 23
comm -13 tier1.txt ruled65.txt | wc -l   # tier 0 only     → 26

# Side check: non-test lines of the ruled 65, less graph/embedding/http_embedder.go (220)
#   → 141,062 − 220 = 140,842, the D4 ceiling
```

## Result

Package paths are relative to `github.com/c360studio/semstreams/`.

**In both (39).** `component`, `component/flowgraph`, `config`, `graph`, `graph/clustering`, `graph/geo/geojson`,
`graph/readiness`, `message`, `metric`, `model`, `natsclient`, `output/websocket`, `payloadregistry`, `pkg/buffer`,
`pkg/errs`, `pkg/fusion`, `pkg/fusion/fusionnats`, `pkg/fusion/fusionvocab`, `pkg/graphview`, `pkg/lifecycle`,
`pkg/projection`, `pkg/retry`, `pkg/types`, `processor/graph-clustering`, `processor/graph-embedding`,
`processor/graph-index`, `processor/graph-ingest`, `processor/graph-query`, `processor/rule`,
`processor/rule/expression`, `service`, `storage`, `storage/objectstore`, `storage/storeregistry`, `types`,
`vocabulary`, `vocabulary/bfo`, `vocabulary/cco`, `vocabulary/export`.

**In tier 0 only (26).** `composition`, `composition/cli`, `graph/embedding`, `graph/inference`, `graph/query`,
`graph/structural`, `health`, `internal/componentadmission`, `internal/graphmutation`, `internal/lifecyclecleanup`,
`internal/logforwarderpolicy`, `internal/maxdelivery`, `pkg/acme`, `pkg/cache`, `pkg/dispatch`, `pkg/platform`,
`pkg/projection/contract`, `pkg/resource`, `pkg/revlag`, `pkg/rulepack`, `pkg/security`, `pkg/timestamp`,
`pkg/tlsutil`, `pkg/worker`, `processor/graph-index-spatial`, `processor/graph-index-temporal`.

**In Tier 1 only (23)**, each with its disposition and where that comes from:

| Package | Disposition | Source |
| --- | --- | --- |
| `agentic`, `agentic/agentrun`, `vocabulary/agentic` | separated: `defer-exclude` row | 03B D4 (agentic domain); task 5.1 |
| `gateway`, `gateway/graph-gateway` | separated: `defer-exclude` row | 03B D4 (layer 4 is consumer-owned); task 5.1 |
| `model/wire` | behind the seam: carried dormant in change 2, leaves in change 7 | 03B D4; foundation D8 |
| `componentregistry`, `payloadbuiltins`, `vocabulary/builtins` | not ported: aggregators; each consumer registers per package | 03B Q1 ruling (#3) |
| `processor/gated-dag` | not ported: reached only through the aggregators | 03B Q1 ruling, measured consequence |
| `processor/agentic-dispatch`, `processor/agentic-loop`, `processor/agentic-loop/lessonmatch`, `processor/agentic-model`, `processor/agentic-tools`, `processor/agentic-tools/executors`, `processor/agentic-tools/runner`, `persona` | not ported: agentic domain | 03B D4 ("`governance` and `processor/agentic-*`"); no tier-0 package imports `persona` |
| `input/websocket` | not in any change of the chain; the change that ports it inherits the ACME loaders' row | `setup-04a-01-floor` D1; #9 comment 5950822861 |
| `gateway/lifecycle-gateway`, `pkg/context`, `pkg/logging`, `test/e2e/mock` | not ported: no tier-0 package imports them (see "Edges leaving the set"), and neither first-wave consumer imports them (below) | this measurement |

Not ported means no ledger row: a row is written for a package that is ported, or one that tier 0 reaches and the
rulings cut (`docs/provenance.md`).

## Edges leaving the set

`go list -f '{{.ImportPath}} {{join .Imports " "}}'` over the ruled 65 finds twelve imports of a SemStreams package
outside the set:

- `component` → `agentic`;
- `graph/clustering`, `graph/inference`, `graph/query`, `processor/graph-clustering` and `processor/graph-query`
  → `graph/llm`;
- `processor/graph-query` → `vocabulary/agentic`;
- `processor/rule` → `agentic`, `agentic/agentrun`, `governance` and `vocabulary/agentic`;
- `service` → `agentic/agentrun`.

Every one ends at a package the rulings cut. These are the port-refactor edges of SETUP 03B design D4 (E1–E4 and
the seam). No tier-0 package imports any of the five unruled Tier-1-only packages, or `input/websocket`.

## Against the first measurement

The counts are the same, 39 / 23 / 26, but the membership changed:

| Group | Joined it under the rulings | Left it under the rulings |
| --- | --- | --- |
| In both | `graph/geo/geojson`, `output/websocket`, `processor/graph-clustering`, `processor/rule`, `processor/rule/expression`, `vocabulary/export` | `agentic`, `agentic/agentrun`, `gateway`, `gateway/graph-gateway`, `model/wire`, `vocabulary/agentic` |
| Tier 0 only | `composition/cli`, `internal/maxdelivery`, `processor/graph-index-spatial`, `processor/graph-index-temporal` | `graph/llm`, `internal/agentterminal`, `internal/deliverylane`, `internal/looptoken` |

The six that left "both" are now Tier-1-only, and the six that joined came from Tier-1-only. This is the change
SETUP 03B design D2 predicted. Every package that left tier 0 is a separated package with a ledger row: task 5.1,
or the carried-dormant rows of change 2.

## Do the first-wave consumers import a Tier-1-only package?

Imports of `github.com/c360studio/semstreams/...` were read from tarballs fetched with `gh api … /tarball/<sha>`.
Both repositories require the pin (`go.mod`: `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`):

- semsource at `e4febc0d2e23bf33811edc20b9a4159cf5275dc4` (the SHA design D5 cites);
- semconnect at `dff1265708910ef411ab6f85b16fd9e94663e997`.

| Consumer | Production imports | Tier-1-only among them | Test-only Tier-1-only |
| --- | --- | --- | --- |
| semsource | 25 packages | `componentregistry`, `payloadbuiltins` | none |
| semconnect | 27 packages | `gateway`, `payloadbuiltins`, `vocabulary/builtins` | `payloadbuiltins` |

Each hit already has a ruling. The aggregators move to per-package registration in each consumer's composition
root (03B Q1). SemConnect's `gateway` import is a compile-time interface assertion
(`gateway/cs-api/component.go:198`), and dropping it is SemConnect work (03B D4). Neither consumer imports a
package outside the ruled 65 and Tier 1.

**Not measured.** semteams' and semboids' imports of `gateway/lifecycle-gateway`, `pkg/context`, `pkg/logging`,
`persona` and `test/e2e/mock`. `docs/inventory-scope.md` admits only symbol-level questions about named seams for
those two repositories. It has no question for these packages.
