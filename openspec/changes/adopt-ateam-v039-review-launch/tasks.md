# Tasks: adopt-ateam-v039-review-launch

Each step ends when `task verify` passes on a clean tree at its commit (#171), with ateam v0.3.9 first on `PATH`. One PR.

## 1. Pin and review launch

- [ ] 1.1 Pin `v0.3.9` in both workflows, the merge-gate tool tests and `.agents/README.md`; copy v0.3.9's
  `handoff` and `review-pr` skill text unedited; `ateam doctor --check` and `TestAteamTemplatesUnchanged` pass at v0.3.9.
- [ ] 1.2 `.agents/review-policy.json` sets `review_launch: agent`; `.agents/hosts/enjoy.toml` is ateam v0.3.9's;
  `ateam review request --dry-run` on this PR prints a Codex reviewer run and the Enjoy launch text.

## 2. Prose and spec

- [ ] 2.1 `.agents/protocol.md` § Cross-agent review says the implementer starts the reviewer; the spec delta applied
  to `openspec/specs/`.
