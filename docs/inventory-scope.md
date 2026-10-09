# Inventory and consumer scope

Which repositories an agent may read, and for what question. Ruled by the owner on
[#22](https://github.com/C360Studio/semengine/issues/22) (2026-10-01). A repository not in this table, or marked
"none", is not inventoried; adding one is an owner ruling on #22, not an agent's call.

## Starter set

SemEngine's consumers at the start are **semsource, semconnect, semteams and semboids**. **semembed and seminstruct
are support services**, not consumers. SemStreams is the source at the pin and retires if this effort succeeds. The
rest of the portfolio is archived, folded into semteams, or post-MVP, and is not a requirement source.

| Repository | SemStreams dependency (2026-10-01) | Role | What may be inventoried from it |
| --- | --- | --- | --- |
| semsource | pin `8b99efe9` | **first-wave consumer, tier 0** | dependency closure and imports; 03A baseline; retained-contract-matrix column; consumer qualification corpus (`test/setup03a`); whether `internal/sourcelifecycle` duplicates `pkg/lifecycle` |
| semconnect | → pin (PR #74) | **first-wave consumer, tier 0** | the same; identity (ADR-102/104), geo and temporal index, typed mutation client, vocabulary/export seams |
| semteams | beta.160 | **named next consumer**; absorbs semdev; home of any UI work | symbol-level use of `processor/rule`, `pkg/lifecycle` and the agentic seams — the qualifying workload for rules and business workflows |
| semboids | beta.160 | **verification consumer** — the measuring fixture the owner runs against each slice as soon as it can | the workload it drives (streams, rates, entities, lifecycle use); symbol-level use of `pkg/lifecycle`, `processor/rule` |
| semembed | provider service | support | provider identity (image, model artifact, dimensions, preprocessing) in baseline evidence — never code |
| seminstruct | service | support | nothing unless a matrix row names it |
| semdev | beta.160 | folding into semteams | only where semteams' code does not yet show a need semdev carries |
| semstreams-ui | not a Go consumer | reference only | whether it consumes `output/websocket` (#8 Q11); nothing else |
| semmem | not a Go consumer | reference for identity rows | the federation identity context ADR-102 cites, if an identity row needs it |
| semstreams | — | the source, at the pin | code facts and decision records (`docs/adr`) only from the `8b99efe9` snapshot with path:line; `main` beyond the pin only to find unmerged work touching port-set packages |
| semsage, semspec, semspec-ui | archived / archiving | none | none |
| semmachina, semdragon, semops, semlink | post-MVP or not returning | none | none |
| semsim, semdocs, semstreams-poly, servicesim, semdev-test, semdev-test-sub, semmem-test | not consumers | none | none |

## Rules

1. An inventory names its question first, then the repositories it will read from this table. A repository marked
   "none" is never grepped for requirements, and none is added mid-inventory.
2. Import counts are not usage. A consumer's needs are measured at symbol level, and only for a consumer this table
   names.
3. One inventory per question, reviewed once by the independent reviewer. A second inventory on the same question
   needs the owner's word.
4. SemStreams code facts and decision records come from the pin snapshot. Sister checkouts under the owner's
   workspace may be live work areas: never modified, never the target of `git` commands; cite with
   `gh api … ?ref=<sha>`.
