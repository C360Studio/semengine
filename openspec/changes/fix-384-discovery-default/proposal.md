# Proposal: fix-384-discovery-default

## Why

`config.Loader.getDefaults` (`config/config.go:491-494`) adds a `discovery` service entry (`Enabled: false`, config
`{}`) to every loaded configuration (#384). No `discovery` service exists in this tree or at the SemStreams pin
`8b99efe9`, so every loaded document names a service no constructor builds. Code that walks `Services` and expects
each name to resolve, such as an operator listing or a composition check, sees an entry that cannot exist.

## What Changes

- The loader's defaults stop naming `discovery`. The `heartbeat` default stays.
- Tests, the `config` README and `config/example_config.json` stop treating `discovery` as a default service, as the
  inventory finds them.

## Capabilities

### Modified Capabilities

- None yet (`skip_specs: true`). The `config-loading` spec does not list the default services; the design settles
  whether it should gain a requirement.

## Impact

- Code: `config/config.go`; `config/*_test.go` as the inventory lists.
- Docs: `config/README.md:48`; the `config` row of `docs/admission-ledger.yaml`.
- Behaviour: a loaded configuration no longer carries a `discovery` entry unless the document names one.
