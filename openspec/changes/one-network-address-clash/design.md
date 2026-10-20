# Design: one-network-address-clash

Accepted by the owner on 2026-10-20 (#328 comment 6290643374), with the rulings under "Owner rulings". Line numbers
are at base `72570e0` unless stated.

## Terms

- **Claim:** an exclusive port's hold on a resource. Only `NetworkPort` is exclusive (`component/port_network.go:18`),
  so a claim is a listener: protocol label, host, port. Inputs and outputs both claim; output network ports are
  listeners (pin `output/websocket/websocket.go:505`, semsource `cmd/semsource/run.go:1034`).
- **Transport:** UDP when the protocol label is `udp` in any letter case, TCP for every other label (rulings).
- **Wildcard host:** an IP literal that is unspecified once an IPv4-mapped form is unmapped: `0.0.0.0`, `::`,
  `::ffff:0.0.0.0`. The codec turns an empty host into `0.0.0.0` (`component/port_codec.go:289-290`).
- **Clash:** two claims whose listeners cannot both bind, under the rule below.

## Owner rulings

- 2026-10-20, #328: "Only \"udp\" is UDP; every other label is TCP"; hosts: "Yes: test both systems and decide it in
  this PR".
- 2026-10-20, #328 comment 6290643374:
  - Q1: "Yes, refuse it everywhere." A wildcard and a specific host on one TCP port are refused on every OS.
  - Q2: "No, ignore case: `UDP` is UDP too." The label is compared with `strings.EqualFold`.
  - Q3: "Yes, add the refused component's name." The refusal names the refused component, the holder and both
    claims, between components and within one.
- 2026-10-20, #339 comment 6291592448 (Codex's MEDIUM on PR #332): "A: always ignore it (simpler; recommended)."
  Admission ignores an IPv6 zone on every address.

## What exists

See `inventory.md`. The registry compares `ResourceID` text in two places: between components
(`component/registry.go:579`) and within one component (`:628`). `ResourceID` is `protocol:host:port`
(`component/port_network.go:14`), and `NetworkFacts` already holds the three parts (`component/port_facts.go:35-40`,
built at `:167`).

## The rule

Two claims clash when they have the same transport and the same port, and one of these holds:

1. Either host is a wildcard.
2. Both hosts are IP literals naming the same address. An IPv4-mapped IPv6 address names its IPv4 address, and
   `0:0:0:0:0:0:0:1` names `::1`. An IPv6 zone is ignored: `::1%lo0` names `::1`, and `fe80::1%en0` names
   `fe80::1%en1` (#339).
3. Both hosts are the same text.

A host name (anything that is not an IP literal) is not resolved.

Evidence (`notes/probe.md`, Go 1.26.9 on macOS 26.5.2 and Linux 7.0.14):

- TCP and UDP on one port bind on both systems, so transport separates claims.
- Go binds every wildcard as one dual-stack `[::]` socket, so wildcards clash with each other everywhere.
- On UDP, a wildcard clashes with every host on both systems. On TCP it clashes on Linux, including with `::1` and
  `localhost`, but not on macOS.
- `127.0.0.1` and `::1` bind together on both systems, and so do `127.0.0.1` and `127.0.0.2` on Linux.
- A zone on an address that is not link-local is ignored by both systems: `::1`, `::1%lo0`, `::1%1` and `::1%en0`
  all bind `[::1]` (`notes/probe-339.md`). On a link-local address the same zone text clashes, and so does an
  interface's name beside its index (`fe80::1%lo0` beside `fe80::1%1`). An unzoned link-local address never binds.

**Where the systems disagree.** On macOS, a wildcard and a specific host on one TCP port both bind. The rule refuses
them anyway, because the same composition fails on Linux (ruled Q1). This is the one case where admission refuses
what one OS allows.

**Not promised.** Admission does not resolve names. So `localhost` beside `127.0.0.1`, or two different names for
one address, pass admission, and the bind at `Start` decides them. Because zones are ignored, one link-local address
on two interfaces (`fe80::1%en0` beside `fe80::1%en1`) is refused, though whether both could bind was not probed
(ruled, #339). Also not promised: listeners opened with
`tcp4`/`tcp6`/`udp4`/`udp6` or `SO_REUSEPORT`, other OSes, other Go versions. The OS never sees the protocol label:
the component maps it to a network, and this rule assumes it maps `udp` (any case) to `udp` and every other label to
`tcp`.

## Options considered

| Option | Cost | Verdict |
| --- | --- | --- |
| Do nothing | Start fails loudly on the cases above; admission exists to catch them | rejected by #328 |
| Normalise `ResourceID` text (`http` becomes `tcp`), keep string equality | `registry.go:375` would stop seeing a protocol-label drift between declared and constructed ports; wildcards still pass (`0.0.0.0` beside `::` fails at Start on both OSes) | rejected |
| Text hosts, transport only | the same wildcard gap | rejected |
| Per-OS rule (`runtime.GOOS`) | whether a composition is valid would depend on where it boots; only two systems probed | rejected |
| Resolve names at admission | predicts a resolver answer that may differ at bind time | rejected |
| **Clash relation over `NetworkFacts`, `ResourceID` text unchanged** | about 40 lines; `net/netip` and `strings` imports | **accepted** |

## Behaviour

- `CreateComponent` refuses a component one of whose claims clashes with a claim an admitted component holds.
  - The error is of the invalid class (`errs.IsInvalid`) and reads:
    `resource conflict: component '<name>' claims <claim>, held by component '<holder>' as <held claim>`.
  - Each claim is shown as its `ResourceID` text.
  - When several admitted components clash, the holder named is the first in instance-name order.
  - The admitted set is unchanged.
- `Declare` and `CreateComponent` refuse a component two of whose own claims clash, with:
  `resource conflict: component '<name>' claims <later claim>, held by another of its ports as <earlier claim>`.
  Claims are ordered inputs first, then outputs, each in declaration order.
- Both texts change from today's; the two existing tests' literals change with them.
- `ResourceID`, `Declaration.ExclusiveResources` (sorted text) and the declared-versus-constructed comparison are
  unchanged. File ports still share.

## Sketch (the developer may differ; the mutants below are written against it)

`component/port_facts.go`, after `NetworkFacts.Port` (`:145`), importing `net/netip` and `strings`:

```go
// transport is the socket type a listener for this port opens: UDP for the
// protocol label "udp" in any letter case, TCP for every other label (owner
// rulings, #328).
func (f NetworkFacts) transport() string {
    if strings.EqualFold(f.protocol, "udp") {
        return "udp"
    }
    return "tcp"
}

// clashes reports whether listeners for f and other cannot both bind: the same
// transport and port, and a wildcard host on either side, one IP address in two
// spellings, or the same host text. Host names are not resolved (probe, #328).
func (f NetworkFacts) clashes(other NetworkFacts) bool {
    if f.transport() != other.transport() || f.port != other.port {
        return false
    }
    a, b := hostAddr(f.host), hostAddr(other.host)
    if a.IsUnspecified() || b.IsUnspecified() {
        return true
    }
    if a.IsValid() && b.IsValid() {
        return a == b
    }
    return f.host == other.host
}

// hostAddr returns the IP address a host spells, unmapping an IPv4-mapped IPv6
// address and dropping any IPv6 zone (owner ruling, #339); for a host name it
// returns the zero Addr.
func hostAddr(host string) netip.Addr {
    addr, err := netip.ParseAddr(host)
    if err != nil {
        return netip.Addr{}
    }
    return addr.Unmap().WithZone("")
}
```

`component/registry.go`, importing `maps`:

- `exclusiveClaims(inputFacts, outputFacts []PortFacts) []PortFacts` returns the `IsExclusive` facts, inputs first.
- `claimsClash(a, b PortFacts) bool` calls `NetworkFacts.clashes` when both have network facts, and otherwise
  compares `ResourceID` text. That fallback exists only for a future exclusive kind, so the check stays closed if one
  is added.
- `exclusiveResources(instanceName, ...)` (`:619`) checks each claim against the earlier ones with
  `if claimsClash(claim, earlier) {` and refuses with
  `fmt.Errorf("resource conflict: component '%s' claims %s, held by another of its ports as %s", instanceName,
  claim.ResourceID(), earlier.ResourceID())`, then returns the sorted `ResourceID` texts.
- `checkResourceConflictsLocked(candidate Declaration)` (`:575`; its call at `:406` passes `prepared`) loops
  `for _, holder := range slices.Sorted(maps.Keys(r.declarations)) {` over each holder's `exclusiveClaims`, with
  `if claimsClash(claim, heldClaim) {`, and refuses with
  `fmt.Errorf("resource conflict: component '%s' claims %s, held by component '%s' as %s", candidate.InstanceName,
  claim.ResourceID(), holder, heldClaim.ResourceID())`.

## Ruling to implementation

Line numbers are at the commit that lands task 2.2. Tests are in `component/registry_boot_admission_test.go` (`T:`
below).

| Ruling | Implemented | Proved |
| --- | --- | --- |
| #328: every label other than `udp` is TCP | `component/port_facts.go:155-160`, `NetworkFacts.transport` | `TestRegistryRefusesNetworkClaimsThatCannotBothBind` (`T:381`) rows R1, R2 (`T:383`, `:385`); `TestRegistryAdmitsNetworkClaimsThatCanBothBind` (`T:439`) rows A1, A2 (`T:441`, `:443`) |
| Q2 (6290643374): `udp` in any letter case is UDP | `component/port_facts.go:156`, `strings.EqualFold` | R3 (`T:387`, `UDP` beside `udp` refused); A8 (`T:455`, `UDP` beside `tcp` admitted) |
| #328 hosts and Q1 (6290643374): a wildcard clashes with any host, on every OS | `component/port_facts.go:170`, `NetworkFacts.clashes`, the `IsUnspecified` branch | R4-R8, R11, R13 (`T:389-397`, `:403`, `:407`); A3-A5 (`T:445-449`) show that specific hosts do not clash |
| One address in two spellings clashes, the wildcard included | `component/port_facts.go:186` (`hostAddr`, `Unmap`) and `:174` (`a == b`) | R9, R10, R13 (`T:399`, `:401`, `:407`) |
| Names are not resolved (not promised) | `component/port_facts.go:176`, `NetworkFacts.clashes`, the text branch | R12 (`T:405`, same text refused), A7 (`T:453`, `localhost` beside `127.0.0.1` admitted) |
| #339 comment 6291592448: an IPv6 zone is ignored on every address | `component/port_facts.go`, `hostAddr` (lines filled at task 2.5's commit) | R14-R20 |
| Q3 (6290643374): between components, invalid class, refused component, holder and both claims named | `component/registry.go:579-594`, `checkResourceConflictsLocked` (`claimsClash` at `:585`, text at `:586`) | `TestRegistryRefusesASecondComponentOnATakenAddress` (`T:192`); R rows, paths (a) and (b) |
| Q3 (6290643374): within one component, through `Declare` and `CreateComponent`, component and both claims named | `component/registry.go:626-641`, `exclusiveResources` (`claimsClash` at `:631`, text at `:632`); `exclusiveClaims` at `:645`; `claimsClash` at `:659` | `TestRegistryRefusesOneComponentClaimingAnAddressTwice` (`T:252`, label row `T:280`); R rows, path (c) |
| One holder named, deterministically | `component/registry.go:581`, the sorted holder loop | `TestRegistryNamesTheFirstClashingHolderByName` (`T:497`) |

## Tests

All in `component/registry_boot_admission_test.go`. The oracle is the literal text and literal outcome of each row,
taken from the probe table and the rulings, never computed from `NetworkFacts`.

- `TestRegistryRefusesASecondComponentOnATakenAddress` (`:191`): its expected text becomes
  `resource conflict: component 'second' claims tcp:127.0.0.1:8080, held by component 'first' as tcp:127.0.0.1:8080`.
- `TestRegistryRefusesOneComponentClaimingAnAddressTwice` (`:250`): its expected text becomes
  `resource conflict: component 'gateway' claims tcp:0.0.0.0:8080, held by another of its ports as tcp:0.0.0.0:8080`.
  It gains a row: input `http` and output `ws` on `0.0.0.0:8080`, expecting
  `resource conflict: component 'gateway' claims ws:0.0.0.0:8080, held by another of its ports as http:0.0.0.0:8080`.
- `TestRegistryRefusesNetworkClaimsThatCannotBothBind` (new). Each row is two `NetworkPort`s with their
  `ResourceID` literals, run three ways. (a) Component `first` is admitted with the row's first claim, then `second`
  is created with its second claim. (b) The same names, with the two claims swapped. (c) One component, `gateway`, has
  the first claim as its input and the second as its output, through `Declare`. Each run checks `errs.IsInvalid` and
  the full refusal text, inside the registry's operation prefixes:
  - (a) `resource conflict: component 'second' claims <second claim>, held by component 'first' as <first claim>`
  - (b) `resource conflict: component 'second' claims <first claim>, held by component 'first' as <second claim>`
  - (c) `resource conflict: component 'gateway' claims <second claim>, held by another of its ports as <first claim>`
  - R1: http `0.0.0.0:8080` / websocket `0.0.0.0:8080`
  - R2: http `0.0.0.0:8080` / tcp `0.0.0.0:8080`
  - R3: `UDP` `0.0.0.0:8080` / udp `0.0.0.0:8080`
  - R4: tcp `0.0.0.0` / tcp `127.0.0.1`
  - R5: udp `0.0.0.0:14550` / udp `127.0.0.1:14550`
  - R6: tcp `0.0.0.0` / tcp `::`
  - R7: tcp `::` / tcp `::1`
  - R8: tcp `0.0.0.0` / tcp `::1`
  - R9: tcp `::ffff:127.0.0.1` / tcp `127.0.0.1`
  - R10: tcp `0:0:0:0:0:0:0:1` / tcp `::1`
  - R11: tcp `0.0.0.0` / tcp `localhost`
  - R12: tcp `localhost` / tcp `localhost`
  - R13: tcp `::ffff:0.0.0.0` / tcp `127.0.0.1` (a wildcard in its IPv4-mapped spelling; Linux refuses)
  - R14: tcp `::1` / tcp `::1%lo0`
  - R15: tcp `::1%lo0` / tcp `::1%1`
  - R16: udp `::1%lo0` / udp `::1%en0`
  - R17: tcp `fe80::1%lo0` / tcp `fe80::1%lo0`
  - R18: tcp `fe80::1%en0` / tcp `fe80::1%en1` (two interfaces; refused by the ruling, not probed)
  - R19: tcp `fe80::1%lo0` / tcp `fe80::1%1` (an interface's name beside its index; both systems refuse)
  - R20: tcp `::%lo0` / tcp `127.0.0.1` (a zoned wildcard is a wildcard)
  - Port 8080 unless shown.
- `TestRegistryAdmitsNetworkClaimsThatCanBothBind` (new, the sensitivity pair). The same three paths (a), (b) and (c)
  with the same instance names; each admits both claims (`Snapshots` holds `first` and `second`; `Declare` succeeds).
  - A1: tcp `0.0.0.0` / udp `0.0.0.0`
  - A2: http `0.0.0.0` / udp `0.0.0.0`
  - A3: tcp `127.0.0.1` / tcp `::1`
  - A4: udp `127.0.0.1` / udp `::1`
  - A5: tcp `127.0.0.1` / tcp `127.0.0.2`
  - A6: tcp `0.0.0.0:8080` / tcp `0.0.0.0:8081`
  - A7: tcp `localhost` / tcp `127.0.0.1` (pins the declared "names are not resolved")
  - A8: `UDP` `0.0.0.0` / tcp `0.0.0.0`
- `TestRegistryNamesTheFirstClashingHolderByName` (new): `beta` on tcp `127.0.0.1:8080` and `alpha` on tcp
  `::1:8080` are admitted. `gamma` on tcp `0.0.0.0:8080` is then created 20 times. A refusal leaves the admitted set
  unchanged, so every attempt meets the same two holders. Every attempt must be refused with exactly
  `resource conflict: component 'gamma' claims tcp:0.0.0.0:8080, held by component 'alpha' as tcp:::1:8080`, and
  `Snapshots` then holds `alpha` and `beta`.

These tests bind nothing, so the fixed-port guard does not apply; the existing tests already use 8080 the same way.

### Mutants (each builds; each run with `task mutate:check` on its named test)

| Mutant | Change | Detected by |
| --- | --- | --- |
| M1 | `if strings.EqualFold(f.protocol, "udp") {` becomes `if !strings.EqualFold(f.protocol, "tcp") {` | R2 |
| M2 | `if a.IsUnspecified() \|\| b.IsUnspecified() {` becomes `if false && (a.IsUnspecified() \|\| b.IsUnspecified()) {` | R4 |
| M3 | `return addr.Unmap().WithZone("")` becomes `return addr.WithZone("")` | R9, R13 |
| M4 | in `checkResourceConflictsLocked`, `if claimsClash(claim, heldClaim) {` becomes `if claim.ResourceID() == heldClaim.ResourceID() {` | R1, paths (a) and (b) |
| M5 | in `exclusiveResources`, `if claimsClash(claim, earlier) {` becomes `if claim.ResourceID() == earlier.ResourceID() {` | R1, path (c) |
| M6 | `for _, holder := range slices.Sorted(maps.Keys(r.declarations)) {` becomes `for holder := range maps.Keys(r.declarations) {` (`slices` is still used at `registry.go:377`) | `TestRegistryNamesTheFirstClashingHolderByName` |
| M7 | `if strings.EqualFold(f.protocol, "udp") {` becomes `if strings.EqualFold(f.protocol, "udp") && f.protocol == "udp" {` (`strings` stays used) | R3 |
| M8 | `return addr.Unmap().WithZone("")` becomes `return addr.Unmap()` | R14, R20 |

M6 makes the named holder depend on map order. The test makes 20 attempts in one run; the reviewer measured unsorted
iteration over the two holders naming `alpha` first in about 1 of 8 attempts (Go 1.26, 100,000 trials; map order is
measured, not promised), so the chance all 20 pass is negligible and one `task mutate:check` run (`-count=1`,
`internal/harness/mutcheck/runs.go:88`) detects it.

**Uncovered, recorded:**

- `claimsClash`'s text fallback: no exclusive kind other than network exists.
- Two different host names for one address: the outcome depends on the resolver, and it is not promised.
- One link-local address on two interfaces (R18): refused by the ruling; neither probe environment could place one
  address on two interfaces, so whether both could bind is not known.

### Generated checks: not used

The rule is a symmetric predicate over two claims plus an add-only history. The examples cover it because:

- **Interacting inputs.** The predicate reads three things:
  - transport: same or different, including two TCP labels, and `UDP` beside both `udp` and `tcp`;
  - port: same or different;
  - host relation: wildcard on either side (zoned or not), one address in two spellings (mapped, long form, any
    zone), two distinct addresses, same name, name beside an address.

  Each combination that changes the outcome is a row: R1-R20 refuse and A1-A8 admit. Each transport branch (M1, M7),
  the wildcard branch (M2), the address branch (M3), the zone drop (M8) and the text branch (A7) is proved
  separately.
- **Symmetry.** Every row runs in both orders, and within one component as well.
- **History.** The registry only adds components, and a repeated name is refused before the clash check
  (`component/registry.go:401-404`). The only histories are "a holder, then a claimant" (both orders per row)
  and "several holders, then a claimant" (`TestRegistryNamesTheFirstClashingHolderByName`).

There is no input format with size limits: host text is parsed by `netip.ParseAddr` only.

## Who should adopt it

`claimsClash` is the one clash rule. When the pin's `composition/validate.go:139-155` is ported (change 3), it must
call it rather than compare `Declaration.ExclusiveResources` text as map keys. Exposing the rule to `composition` is
that port's design. One tracking issue is filed (task 3.1).

## Docs and ledger

- `component/README.md:208-209` and `:215`: the refusal is for claims that clash, with the rule in one sentence and
  "host names are not resolved".
- `component/port_network.go:7`: the `Protocol` comment says `udp` in any letter case means UDP and any other label
  means TCP.
- `docs/admission-ledger.yaml`, `component` row: adaptation (13), in `one-network-address-clash` (#328): "exclusive
  network claims clash by transport (`udp` in any letter case, else TCP), port and host (wildcard, one IP address, or
  same text), not by resource text; the refusal names the refused component, the holder and both claims".
  `proving_tests` gains the three new tests. No `carry` row changes.

## Merge order

- #329 (config) and #337 (rulepack) share no file; whichever lands second merges `origin/main`.
- #280 item 2 would join `Declaration`'s port and facts lists, which this check reads; whichever lands second adapts
  `exclusiveClaims`'s callers.

## Not in scope

- Resolving host names.
- Any non-network exclusive kind.
- `composition`'s check (adoption, above).
