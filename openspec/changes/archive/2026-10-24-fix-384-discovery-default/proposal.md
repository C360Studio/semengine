# Proposal: fix-384-discovery-default

## Why

`config.Loader.getDefaults` (`config/config.go:491-494`) adds a `discovery` service entry (`Enabled: false`, config
`{}`) to every loaded configuration (#384). No `discovery` service exists in this tree or at the SemStreams pin
`8b99efe9`, so every loaded document names a service no constructor builds. `config.Manager` writes it to the
configuration bucket as `services.discovery` on every push, and the service manager keeps it in its boot copy of the
service configs. It is skipped only because it is disabled: enabling it, as `config/example_config.json:13` does, fails
composition with "no constructor registered for service discovery". No test notices the entry:
`TestLoader_Defaults` asserts that it is disabled, which a missing entry also satisfies.

## What Changes

- The loader's defaults stop naming `discovery`; the `heartbeat` default stays.
- A new test pins the default service set to exactly `heartbeat`, and the always-true `discovery` assertion in
  `TestLoader_Defaults` goes.
- `config/example_config.json` stops enabling the unbuildable `discovery` service.
- The doc comment on `types.ServiceConfigs` (`types/service.go:47-50`) stops naming `discovery` and stops saying
  constructors register in `init()`.

## Capabilities

### Modified Capabilities

- `config-loading`: ADDED requirement "The default services are the heartbeat alone".

## Impact

- Code: `config/config.go`, `config/example_config.json`, `types/service.go` (comment);
  `config/load_test.go` and `config/config_test.go`.
- Docs: `config/README.md:48`; the `config` row of `docs/admission-ledger.yaml`.
- Behaviour: a loaded configuration no longer carries a `discovery` entry unless the document names one. A bucket
  written before the fix still holds `services.discovery`; under the no-migration ruling, deployments start fresh.
