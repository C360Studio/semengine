# Proposal: feat-745-endpoint-dimensions

Approved-by: cglusky (2026-11-24) <https://github.com/C360Studio/semengine/issues/745#issuecomment-7130928576> sha256:21419b89a559

Track: `quick`. No spec delta: no capability spec covers `model.EndpointConfig`'s fields (`skip_specs`). Package:
`model`; docs in `processor/graph-embedding/MIGRATION.md` and `model`'s doc comments.

## Why

`httpembedder.Config.Dimensions` lets a product declare its embedding model's vector width, so a restart keeps every
stored vector. A product that reads its embedding endpoint from the model registry has nowhere in configuration to
put that width. `model.EndpointConfig` (`model/registry.go:239-290`) carries `provider`, `url`, `model` and
`query_prefix` for the embedding capability, but no width. SemSource builds its `httpembedder.Config` from the
resolved endpoint, so today it would have to hard-code `Dimensions: 384` next to a model name that lives in
configuration. A model change made in configuration alone would then fail every embed (#745).

## What changes

- `EndpointConfig` gains `Dimensions int` (`json:"dimensions,omitempty"`). It is the vector width of an embedding
  endpoint's model, and is used only by the embedding capability, like `query_prefix`. 0, the default, means not
  declared. Other capabilities ignore it.
- `validateEndpoint` (`model/registry.go:517`) refuses a negative value, naming the endpoint, the same way it refuses
  a negative `max_tokens` (`:527-529`).
- `processor/graph-embedding/MIGRATION.md:31-32` says to pass the endpoint's `Dimensions` as
  `httpembedder.Config.Dimensions` along with URL, Model, APIKey and QueryPrefix.

## Non-goals

- Mapping the endpoint to an `httpembedder.Config` inside SemEngine. The product's `main` still builds the config.
- Any check that the declared width matches the model. `httpembedder` already refuses every reply of another width,
  naming both.
- Any request at load.

## Impact

One new optional key. A configuration without it loads and behaves as before. SemSource would set
`"dimensions": 384` on its embedding endpoint and pass it through. One PR (#746), `Closes #745`.
