# Proposal: config-key-letter-case

## Why

`config` loads a document by merging it over the loader's defaults as a JSON map before decoding into `Config`. A key
written in another letter case than the one `Config` declares (`"NATS"`, `"URLs"`, `"Components"`) survives the merge
as a second key. `encoding/json` matches field names regardless of case, so the key is dropped silently when the
defaults carry the canonical spelling, and applied silently when they do not (#325). The owner ruled on 2026-10-20
(#325 comment 6294481629) that loading refuses such a key, naming both spellings.

## What Changes

- `LoadFile` and `LoadFromBytes` refuse a document holding a key, at any level the loader decodes into a known
  struct, that equals a declared key except for letter case, with an error naming the key as written and as declared.
- The `config` docs say keys are written exactly as shown.

## Capabilities

- Modified: `config-loading`.

## Impact

- `config/config.go` and its tests; `config/README.md` and `config/doc.go`.
