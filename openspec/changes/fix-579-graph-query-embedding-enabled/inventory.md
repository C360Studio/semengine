# Inventory: fix-579-graph-query-embedding-enabled

base: 88aa6033 (branch claude/graph-query-embedding-enabled; origin/main 9f7f5adc plus the claim commit)

Owner rulings: #579, two comments of 2026-11-12 (code `not_enabled`, class invalid, names the verb; key
`embedding_enabled`, on when left out; the no-embedder smoke stack sets it false). Carries no package from the pin;
category 6 is "none: this change carries no pin package".

## 1. The claimed gap

- `git grep -n -i "embedding_enabled\|EmbeddingEnabled\|not_enabled\|NotEnabled"`: no hits. No key, no code.
- `processor/graph-query/config.go:22-29` — `type Config struct` holds only `Ports` and `QueryTimeoutMs`.

## 2. Every current spelling of "this composition has no embedder"

- `processor/graph-query/query.go:33-34` — `"semantic":  {graph.ResponderGraphEmbedding, "search"},` /
  `"similar":   {graph.ResponderGraphEmbedding, "similar"},`: always forwarded.
- `processor/graph-query/query.go:149-160` — `func (c *Component) ask(...)`: when nothing answers, the result is
  transient `internal` naming the subject. Today this is the only signal; the owner ruled it is not to be read as
  "not enabled".
- `GRAPH_STATUS` key `graph-embedding` (`readiness.KeyGraphEmbedding`) is absent on a stack without it
  (`internal/smoke/noembedder_integration_test.go:111-113`); graph-query reads no readiness key.
- `internal/smoke/stack_integration_test.go:102-124,153-160` — `graphOnlyConfig` / `graphOnlyStack`: the composition
  leaves graph-embedding out.
- `openspec/specs/composition-validation/spec.md:311` — `A second responder on a declared verb is an error finding`:
  checks for a second responder, not a missing one. Nothing in the tree checks for a missing one.

## 3. Adjacent claims

- Specs: `openspec/specs/graph-query/spec.md:190-232` (Refusals that are not errors; Configuration is strict and read;
  Derived-index replies are forwarded unchanged); `openspec/specs/composed-stack/spec.md:93-106` (The graph stack
  answers fusion without an embedder) and `:7` (Purpose); `openspec/specs/graph-embedding/spec.md:158-164`
  (`embedding_unavailable`, per entity).
- Codes: `graph/mutation_responses.go:5-17` (who sends each code), `:89-103` (`ErrorCodeEmbeddingUnavailable`);
  `graph/query_verbs_test.go:202` (wire-value pin for `revision_not_reached`).
  `openspec/specs/error-classification/spec.md` lists classes, not graph codes (requirements at :11-62).
- Docs that state today's behaviour: `pkg/fusion/README.md:124-126`, `pkg/fusion/MIGRATION.md:128-131`,
  `internal/smoke/doc.go:14-16`, `docs/admission-ledger.yaml:4828-4830` (fusionnats row, "until then"),
  `processor/graph-query/README.md:33,300-318`, `processor/graph-query/MIGRATION.md:125,162-163`,
  `.agents/skills/query-pattern/SKILL.md:31`.
- Superseded ruling: PR #577 comment 6749007774 (Q-B, transient internal), cited at
  `internal/smoke/noembedder_integration_test.go:184-186`.
- Open PRs (`gh pr list --state open`): #603 (this claim, no files yet); #597, #598 (go.mod, go.sum only). No overlap.
- Open issues naming graph-query: #563 (searchGraph semantic fallback; once ported it must respect the key, so it stays
  open); #487 (tier 2 synthesis), #602 (generation on replies): no overlap.

## 4. Who it is for

- `embedding_enabled`: its caller in this repo is `internal/smoke` `graphOnlyConfig`. Its contract line is the new
  graph-query requirement. SemSource composes graph-embedding and needs no change (owner).
- `graph.ErrorCodeNotEnabled`: sent by `processor/graph-query`; read by `internal/smoke`; offered by the graph-query spec
  requirement.

## 5. Problem shape: refuse at a seam before any request is sent

- Closest instance: `processor/graph-query/query.go:233-236`: `relationships` refuses an unknown direction
  `invalid_request` before any request goes out. This change follows that shape: classified invalid, nothing sent,
  no error counted.
- Default-when-absent config field: `config.go:89-96` `ApplyDefaults` (`QueryTimeoutMs == 0`, `Ports == nil`);
  nil-is-refused: `config.go:58`. Existing `*bool` config fields with default true: none
  (`git grep -n "\*bool" -- '*.go'` only finds schema types and a decode struct at `globalsearch.go:224`). The schema
  generator reads type and default from the tag (`component/schema_tags.go:675-680`, `case "bool"`), not from the Go
  type, so `*bool` with `type:bool,default:true` generates `true`.

## Adopter seam

1. What must a config author know? One fact: set `embedding_enabled: false` when the composition leaves graph-embedding
   out.
2. If they do nothing: either today's behaviour (transient `internal` naming `graph.embedding.query.search`, retried in
   vain), or, with the key set false beside a running graph-embedding, an immediate invalid `not_enabled` naming the
   verb. Neither answers wrongly or empty.
3. Where they find out: a typed runtime error on the first word question.
4. What they should have to know: nothing; graph-query could notice the missing responder itself. The owner ruled
   against inferring it from no-responders (#579). The key stays the one thing to learn.
