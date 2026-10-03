# ADR-104: The Platform Authority Is Unique by Default

> **Ported from SemStreams.** This record is `docs/adr/104-unique-platform-authority.md` at SemStreams commit
> `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. It came with `pkg/types` and `pkg/platform`: it explains why the
> `platform` position of an entity ID (`pkg/types/entity_id.go`) carries a suffix the framework mints. Its
> admission-ledger row is `docs/adr/104-unique-platform-authority.md` in `docs/admission-ledger.yaml`. The text is
> the pin's apart from line wrapping and the corrections marked **Correction (SemEngine port)**, each citing the pin
> line it supersedes. Read it with these facts about this repository:
>
> - Issue and pull request numbers (`#1168`, `#1178`, `#1188`, `#1192`, `#1194`, `gh#459`) are SemStreams', not
>   SemEngine's.
> - The mechanism this record decides lives in `config.Manager.Start`, which is not yet ported (SemEngine change 3,
>   `config`). Bucket and key names such as `semstreams_config` and `platform_identity` are the pin's.
> - ADR-102 (`docs/adr/102-entity-id-segment-semantics.md`) and the `entity-id-contract` spec are ported; the
>   `component-runtime-config` and `framework-bucket-catalog` specs and the `docs/proposals/` inventory are not yet
>   ported. Where the text below calls `entity-id-contract` a capability spec, it is the pin's; here it is the
>   reference contract `docs/specs/entity-id-contract.md`, and its requirements become a capability spec as the code
>   they describe is ported (#72 ruling, 2026-10-03).

## Status

**Accepted (2026-08-31)** — owner endgame instruction on PR #1178 ("we can archive and push; once ci green we can
merge"); every decision individually ruled on #1168 (scope cut + knobless 2026-08-30; "concur with option 1" comment
5479005060). Amends ADR-102 (decision 7's "pre-v1 or never" now applies to a value the *framework* mints, not only
one the operator wrote) by reference. Supersedes nothing.
Mechanics live in the `entity-id-contract` and `component-runtime-config` capability specs.

Scope note: the run-identity half of the reviewed draft (a framework family whose instance digests its full origin)
left this ADR with the owner scope cut of 2026-08-30 and belongs to **#1192**; the import-lane admission fact
belongs to **#1194**. Neither is decided here.

## Context

`platform.org` / `platform.id` are positions 1–2 of every identity a deployment mints (ADR-102). They were validated
for shape and byte budget only, so two deployments provisioned from one configuration template mint under the same
authority — and ADR-102 d7 forbids ever rewriting a minted authority, which makes the collision permanent rather
than repairable. The only detector was `local_authority_claimed`, which fires after two deployments have already
collided inside one graph.

A first-boot persist-and-compare seam already existed (`config.Manager.Start` against the `semstreams_config`
bucket), but it persisted a value the file held and never minted one. Inventory:
`docs/proposals/gh1168-federation-identity-inventory.md`. Owner ruling 2026-08-30: greenfield — no deprecation, no
alias, no parallel path; sisters handle their own migration.

## Decision

1. **`platform.id` is unique by default.** On a deployment's genuine first boot the framework mints a six-hex-byte
   entropy suffix from `crypto/rand`, records it once with an atomic `Create`, and adopts it on every later boot and
   in every co-process sharing that configuration bucket. Nothing in the configuration document disables the mint.

2. **The durable record is keyed on the stem and is not configuration.** `semstreams_config/platform_identity`
   carries exactly `{"org": …, "stem": …, "id": …}` — the declared authority, the declared identifier, and the
   effective identifier. It is created once and never rewritten, never pushed by configuration synchronization,
   never applied back into memory by it, and never watched. The KV `platform` **config** key becomes a read-only
   mirror: still published for the UI, never applied back over the running authority.

3. **Identity is established before arbitration, from one read, in three branches.** The record present → adopt it
   (refusing unless the record's `org` matches and the file's `platform.id` equals the record's `stem`).
   The record absent and the bucket otherwise empty → mint and `Create`. The record absent and other keys present →
   **refuse Start naming that cause, minting nothing and creating nothing**: such a bucket predates identity minting,
   and minting into it would durably record an authority the deployment's own guard then rejects for the wrong
   reason.

4. **The opt-out is the record, not a configuration key.** An operator who owns global uniqueness pre-creates
   `platform_identity` with `id == stem`; the adopt branch takes it and validates it exactly as it validates a
   configuration value. A boolean in the configuration document was rejected: the document is the very artifact the
   threat model says gets cloned, so a cloned opt-out recreates the footgun the mint exists to close (owner ruling,
   2026-08-30).

5. **The authority-pair budget reserves the suffix — at the declaration boundary, and only there.** Configuration
   load bounds `len(org) + len(id) + 7` against the family-table budget, so a pair that fits only unsuffixed cannot
   be durably minted and then rejected forever. A declared pair may therefore be at most **163** bytes.

   The reserve is a fact about a *declaration*, not about a pair. An **effective** pair — the minted identifier, an
   adopted record's, or the running configuration's — already carries whatever suffix it will ever carry and is
   bounded at the full family-table budget of **170**. Applying the reserve to both kinds reserves the same seven
   bytes twice and refuses, at Start, a declaration that had already passed load. Declarations ≤ 163, effective
   pairs ≤ 170; no path admits what another rejects, because no path sees both kinds.

   **Configuration declares the stem, and only the stem.** That is what makes the previous sentence true: while the
   `platform.id` field admitted either a stem or a minted identifier, one field carried both kinds and the boundary
   contradicted itself — at the legal edge a 163-byte stem mints to a 170-byte identifier, so writing that
   identifier back into the file was refused at load and could never reach the adopt branch that claimed to accept
   it. A configuration that declares the minted identifier is refused with guidance naming the stem, decided by
   comparison against the recorded value rather than by inspecting the string's shape.

6. **A bucket that can evict the identity is refused before anything is minted, and the guarantee lives in the
   catalog.** The shared configuration bucket now carries a framework RETENTION guarantee, which is what the
   `framework-bucket-catalog` contract says puts a bucket in its descriptor table. It is catalogued as operational,
   **owner-only**, History 5, with a **strict** no-lifecycle retention kind:
   acquisition verifies that no TTL and no binding size cap is in force and fails closed, and does NOT reconcile the
   policy in place.

   Strict rather than the existing reconciling kind because repair is the wrong answer here: stripping a TTL fixes
   the policy while saying nothing about the keys it already deleted, so a create-once identity may be gone and the
   next boot would mint a *second* authority that decision 7 of ADR-102 forbids ever reconciling. Both writers of the
   bucket — the config manager and the rule ConfigManager — resolve that one descriptor, so the guarantee no longer
   depends on which of them creates it first.

   Owner-only is about who may NOT write, not who may. A generic rule `update_kv` into `platform_identity` plain-Puts
   over the create-once record, and a boot ADOPTS a recorded identifier after checking only its segment grammar and
   byte budget — so a rule pack could move the authority every entity is minted under, or, with a mismatched value,
   prevent the deployment from booting again. The ownership predicate is consulted by nothing but the `update_kv`
   guards, so declaring it costs the legitimate writers nothing.

   **Owner-ruled 2026-08-31** ("concur with option 1", #1168 comment 5479005060), over the alternative of keeping the
   bucket write-open with a key-level refusal for the identity pair: once identity correctness state shares a bucket,
   letting a generic rule writer onto the rest of the configuration plane is the wrong boundary, and the existing
   predicate closes the whole class without a remembered-key list. Configuration changes go through the config
   manager/API; a rule pack that needs to influence configuration is an engine-gap conversation, not a raw KV write.
   The bucket is acquired under the lifecycle context, never one a constructor invented.

7. **At most one `platform.environment` may establish against one bucket.** The pair `(org, id, environment)` was
   compared only on the subsequent-boot branch, so two deployments that both found the bucket empty — one `prod`,
   one `dev` — each took the first-boot branch and published configuration over the other's. An internal guard key,
   claimed by atomic create before the identity record, decides it with the same primitive that decides the record;
   a mismatch refuses Start naming both environments. The guard is **not** a field of the record: `{org, stem, id}`
   is a cross-repo read contract and does not change. Claiming before the record means a failure between the two
   leaves a state a same-environment boot completes and a different-environment boot is refused.

   > **Correction (SemEngine port): decision 7 is superseded and kept here as history** (supersedes pin
   > `docs/adr/104-unique-platform-authority.md:94-100`). The environment guard it describes does not exist at the
   > pin. SemStreams #1188 names the configuration bucket by the declared `(org, stem)` pair; this record's own last
   > consequence says the mechanism is correct without a guard because the adopt branch performs its own comparison
   > (pin `:127-128`). At the pin `platform.environment` separates no deployments: no bucket name, key, guard or
   > comparison reads it, it is a startup log label only, and two deployments declaring the same `platform.org` and
   > `platform.id` are one deployment and share one configuration (pin
   > `openspec/specs/component-runtime-config/spec.md:369-371`). Give each deployment its own `platform.id` instead.

## Consequences

- BREAKING, in the beta.163 wave: every deployment's `platform.id` gains a suffix on its next boot against fresh
  storage, so every entity it mints moves. Fresh storage, no migration (ADR-102 d7).
- **A bucket carried over from before this change refuses Start, loudly, and creates nothing** — the refusal names
  the pre-identity bucket and instructs fresh storage. It is repeatable, not a wedge.
- Adopter fixtures and e2e stop predicting the pair from a configuration file and read
  `semstreams_config/platform_identity` instead — the framework observes; nobody computes.
- The declarable authority pair loses 7 bytes of headroom: 163 rather than 170. The bound on an effective pair is
  unchanged at 170.
- **A configuration bucket carrying a TTL or a size cap no longer boots.** That is a behaviour change for any
  deployment whose bucket was provisioned with one — deliberately, because such a bucket cannot hold a create-once
  identity. Provision it with no TTL and no MaxBytes.
- **A bucket serves one environment.** Two deployments sharing `org` and `platform.id` but differing in
  `platform.environment` can no longer both start against one bucket; the second is refused. This carries the
  environment distinction after #1188 retires the gh#459 config-key guard, which is the only place it lived before —
  and it holds on the first-boot branch, where that guard never ran.

  > **Correction (SemEngine port):** superseded with decision 7 (supersedes pin `:115-118`). At the pin a bucket is
  > named by `(org, stem)`, and two deployments that differ only in `platform.environment` are one deployment.
- The shared configuration bucket is acquired inside `Start(ctx)`, which also rejects a nil context. A caller that
  used a bucket-dependent method before Start now receives a named error instead of operating on a bucket the
  constructor had opened behind it.
- **A Start that fails leaves no writer armed.** The acquired handles reach the exported write methods only when
  Start completes; every refusal — retention, foreign identity, pre-identity bucket, lost environment claim,
  watchers — leaves `PushToKV` and the component writers returning the not-acquired error. A caller that wrote
  through the manager after a failed Start was, until this change, overwriting a bucket the framework had just
  refused as another platform's.

  > **Correction (SemEngine port):** the "lost environment claim" refusal went with decision 7 (supersedes the clause
  > at pin `:123`); the other refusals stand.
- #1188 namespaces this bucket by the pre-mint `(org, stem)` and retires the gh#459 guard; the mechanism above is
  correct without that guard, because the adopt branch performs its own comparison.

## Alternatives rejected

- **A configuration knob** (`platform.unique: true`, `platform.mint_suffix: false`) — cloned N times it recreates the
  shared authority; and an unknown-key typo is silently dropped by `encoding/json`, so it would also require a strict
  `platform`-block key check whose only motivating hazard is the knob itself.
- **An environment override** — forks identity between co-processes sharing one bucket.
- **Minting at config load and rewriting the file** — read-only mounts fail; two hosts sharing a config diverge.
- **Deriving from a host fact such as the hostname** — clones with equal hostnames still collide; not entropy.
- **Refusing an entropy-less identifier at load** — "entropy-less" is undecidable by grammar, and it moves the mint
  into the operator's hands.

## Cross-repo contract

A sister conforms when: its composition root passes `deps.Platform` unchanged; its configuration files declare the
STEM and accept the minted suffix, or it pre-creates `platform_identity` for a deployment that must stay unsuffixed;
its configuration bucket carries no TTL and no size cap; one environment per bucket; and its fixtures,
e2e and tooling read the effective pair from `semstreams_config/platform_identity` rather than predicting it from a
configuration file. The record's shape is normative in the `component-runtime-config` capability spec: exactly the
three fields `org`, `stem`, `id`.

> **Correction (SemEngine port):** "one environment per bucket" went with decision 7 (supersedes the clause at pin
> `:145`). A sister separates deployments by giving each its own `platform.id`, not its own `platform.environment`.
