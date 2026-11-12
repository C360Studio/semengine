# Design: fix-579-graph-query-embedding-enabled

## Context

The accepted inventory is `inventory.md` (base 88aa6033). The premises this design rests on:

- graph-query always forwards `semantic` to `graph.embedding.query.search` and `similar` to
  `graph.embedding.query.similar` (`processor/graph-query/query.go:33-34`). With no graph-embedding, `ask`
  (`query.go:149-160`) returns transient `internal` naming the subject.
- The config is decoded strictly, then defaults are filled in, then it is validated, in one function shared by the
  port declarer and the factory (`processor/graph-query/component.go:67-93`). `ApplyDefaults` fills zero values
  (`config.go:89-96`); `Validate` refuses a nil `Ports` (`config.go:58`).
- Nothing in the tree spells the key or the code (`git grep -n -i "embedding_enabled\|not_enabled"`: no hits).
- Graph error codes are declared in `graph/mutation_responses.go:18-124`, and its doc comment (`:9-17`) says who sends
  each one. `error-classification` lists classes, not codes.

## Goals / Non-Goals

Goals: one key, on when left out; `semantic` and `similar` refuse `not_enabled` when it is off; the smoke stack proves
it. Non-goals: inferring the setting from no-responders, `GRAPH_STATUS` or the composition (ruled out, #579);
changing any other verb; `searchGraph`'s semantic fallback (#563, not in the tree; when it lands it reads this key).

## Decisions

### D1. How "on when left out" is represented

- **A (chosen): `EmbeddingEnabled *bool`**, `json:"embedding_enabled,omitempty"`, schema tag
  `type:bool,description:...,default:true,category:basic`. `ApplyDefaults` sets nil to true, as it does for the
  timeout; `DefaultConfig` sets true; `Validate` refuses nil, as it refuses nil `Ports`, so the component never reads
  a config whose defaults were not filled in. `null` decodes to nil and is the same as leaving the key out (how
  `encoding/json` decodes a pointer). A string such as `"false"` is refused by strict decoding. Cost: a pointer in an
  exported struct. Hand-built `Config` values must now set it, which only tests do.
- B: plain `bool`, decoding onto `DefaultConfig()`. A written `ports` block would decode into the default one and
  merge, which ruling K forbids (`config.go:86-88`). Rejected.
- C: plain `bool` with a custom `UnmarshalJSON` that notices the key is present. More code than A for the same result.
- D: invert it to `embedding_disabled`, false by default. Contradicts the ruled key name.

Adopter seam: one fact (set it false without graph-embedding). Doing nothing keeps today's behaviour, a typed transient
refusal. No silent loss is possible.

### D2. Where the refusal happens

- **A (chosen):** the handler table (`query.go:23-80`) keeps `semantic` and `similar` subscribed. When the key is off,
  each gets a handler that counts the input and returns
  `errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeNotEnabled, ...)` without reading the request or calling
  `ask`. The message names the verb's graph-query name (`semantic` or `similar`) and the key, for example
  `semantic is not enabled: graph-query's embedding_enabled is false`.
- B: leave the two verbs unsubscribed. The caller would get a no-responders error, which says nothing about why, and
  `Start`'s every-declared-verb check (`query.go:85-90`) would need an exception. Rejected.

Counting: like `invalid_request` and a responder's refusal, `not_enabled` adds one input in `DataFlow` and no error in
`Health` ("Refusals that are not errors", spec delta). It is a fact about the composition, not a failure of graph-query.

### D3. The code

`graph.ErrorCodeNotEnabled = "not_enabled"` goes beside `ErrorCodeEmbeddingUnavailable` in
`graph/mutation_responses.go`, with a doc comment saying it means the composition turned the verb off, and how it
differs from `embedding_unavailable` (one entity) and `index_not_ready` (an index catching up). The doc comment at
`:9-17` gains "not_enabled is sent by processor/graph-query's semantic and similar verbs". Its wire value is pinned
the way `revision_not_reached` is (`graph/query_verbs_test.go:202`).

## Invariants

- For every request bytes (any content, malformed included), with the key off, `semantic` and `similar` return invalid
  `not_enabled` and send nothing (spec delta, "Embedding verbs can be turned off").
- With the key on or left out, every verb behaves byte for byte as today ("Derived-index replies are forwarded
  unchanged", modified).

## Proving tests

- Unit, `processor/graph-query/config_test.go`:
  - `TestConfigEmbeddingEnabled`: left out, `null` and `true` give true; `false` gives false; `"false"` is refused;
    shown to fail if `ApplyDefaults` stops setting it.
  - `TestValidateRefusesHandBuiltConfigs` gains a nil case.
  - `FuzzDeclarePorts` gains a `false` seed.
- Unit, `TestEmbeddingVerbsRefuseWhenNotEnabled` (`lifecycle_test.go`, which builds handlers through
  `createFromJSON`):
  - With `{"embedding_enabled": false}`, `semantic` and `similar` return invalid `not_enabled` naming the verb.
  - With `{}`, the same calls return the forward's transient `internal` naming the downstream subject. The test
    client has no connection, so `ask` gets `natsclient.ErrNotConnected`.
  - `TestEveryDeclaredVerbHasAHandler` passes for both settings.
- Integration, `TestNotEnabledRefusesBeforeForwarding` (`embedding_integration_test.go`):
  - A real graph-embedding is started, and a direct `graph.embedding.query.search` is awaited until it answers (the
    event that shows it is serving).
  - graph-query is then started with `false`. `semantic` and `similar` are refused `not_enabled`; a forward would have
    been answered.
  - `DataFlow` counts each request; `Health` shows no error.
- Integration, `TestSmoke_NoEmbedderAnswers`: `graphOnlyConfig` sets `"embedding_enabled": false`, and the NL `Fuse`
  refusal is invalid `not_enabled` naming `semantic`. The fullStack test is unchanged (it leaves the key out and
  `semantic` still answers).

## Risks / Trade-offs

- An operator who sets `false` beside a running graph-embedding loses word search, but loudly, on the first request.
- graph-query does not check the key against the composition. A composition-validation finding could do that later;
  no issue is filed, since #579's ruling makes the key the operator's statement.

## Migration Plan

None required. `processor/graph-query/MIGRATION.md` and `README.md` list the new key as optional.

## Open Questions

None for the owner: #579 answers the key, default, code, class, message and smoke setting. That the refusal adds no
error in `Health` follows from the existing "Refusals that are not errors" requirement (D2).
