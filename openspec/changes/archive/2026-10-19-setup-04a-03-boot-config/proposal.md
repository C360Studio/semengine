# Proposal: setup-04a-03-boot-config

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159), in the order the
owner set on 2026-10-19 (#9 comment 6270875286). `config` is fourth. At 5,193 pin lines it is over the ~3,000-line
cap, so the owner ruled on 2026-10-19 (#9 comment 6274501134) that it ports as two pull requests. This change is the
first: the `Config` type, its loading and validation, and the JetStream stream declarations and provisioning that
`Config.Validate` depends on, waived by count at about 3,150 pin lines (3,148 measured). The KV-backed
`config.Manager` (`manager.go`, `key_family.go`, `bucket_name.go`, with #17) is the second pull request.

## What Changes

- Port from SemStreams `8b99efe9`:
  - `config.go`, `security.go`, `streams.go` and `doc.go` as **adapt**: dead surface dropped, the `SEMENGINE_`
    environment prefix (#69), true text, and the owner's answers to Q1 and Q2;
  - `stream_bounds.go` and `stream_drift.go` as **carry**, byte-equal to the pin;
  - their tests, with `natsfixture` for the integration tests (#195), plus `example_config.json` and a trimmed
    `README.md`.
- Not ported:
  - `helpers.go`, `env.go` and `minimal_config.go`: no non-test caller at the pin or in a consumer (owner ruling,
    #9 comment 6274501134);
  - `config/rules/agentic-memory/`, `testdata/`, `example_test.go`, `env_test.go`, `integration_test.go`.
- If the owner answers Q1 yes: loading refuses the NATS settings nothing applies yet (username, password, token,
  TLS, reconnect settings, three JetStream keys), in the file or in the `SEMENGINE_` environment, and the pull
  request carries `Closes #74`.
- If the owner answers Q2 yes: a document that does not decode fails to load instead of falling back to defaults.

## Capabilities

- New: `config-loading` (`specs/config-loading/spec.md`).

## Impact

- New package `config` (without `Manager`).
- Three admission-ledger rows: `config` (adapt), and `config/stream_bounds.go` and `config/stream_drift.go` (carry).
- A `docs/repository-map.md` row, with the entry count raised by three.
- The `config` coverage target (80%, merged profile) is added in the second pull request.
