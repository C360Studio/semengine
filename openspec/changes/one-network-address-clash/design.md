# Design: one-network-address-clash

Draft for inventory review, design review and owner acceptance. Line numbers are at base `72570e0` unless stated.
Rulings: #328, 2026-10-20 (transport; hosts decided here from a probe on macOS and Linux).

## Terms

- **Claim:** an exclusive port's hold on a resource. Only `NetworkPort` is exclusive (`component/port_network.go:18`),
  so a claim is a listener: protocol label, host, port. Inputs and outputs both claim; output network ports are
  listeners (pin `output/websocket/websocket.go:505`, semsource `cmd/semsource/run.go:1034`).
- **Transport:** UDP when the protocol label is exactly `udp`, TCP for every other label (ruling).
- **Wildcard host:** an IP literal that is unspecified once an IPv4-mapped form is unmapped: `0.0.0.0`, `::`,
  `::ffff:0.0.0.0`. The codec turns an empty host into `0.0.0.0` (`component/port_codec.go:289-290`).
- **Clash:** two claims whose listeners cannot both bind, under the rule below.

## What exists

See `inventory.md`. The registry compares `ResourceID` text in two places: between components
(`component/registry.go:578`) and within one component (`:628`). `ResourceID` is `protocol:host:port`
(`component/port_network.go:14`), and `NetworkFacts` already holds the three parts (`component/port_facts.go:35-40`,
built at `:167`).

## The rule

Two claims clash when they have the same transport and the same port, and one of these holds:

1. Either host is a wildcard.
2. Both hosts are IP literals naming the same address. An IPv4-mapped IPv6 address names its IPv4 address, and
   `0:0:0:0:0:0:0:1` names `::1`.
3. Both hosts are the same text.

A host name (anything that is not an IP literal) is not resolved.

Evidence (`notes/probe.md`, Go 1.26.9 on macOS 26.5.2 and Linux 7.0.14):

- TCP and UDP on one port bind on both systems, so transport separates claims.
- Go binds every wildcard as one dual-stack `[::]` socket, so wildcards clash with each other everywhere.
- On UDP, a wildcard clashes with every host on both systems. On TCP it clashes on Linux, including with `::1` and
  `localhost`, but not on macOS.
- `127.0.0.1` and `::1` bind together on both systems, and so do `127.0.0.1` and `127.0.0.2` on Linux.

**Where the systems disagree.** On macOS, a wildcard and a specific host on one TCP port both bind. The rule refuses
them anyway, because the same composition fails on Linux. This is the one case where admission refuses what one OS
allows (owner question 1).

**Not promised.** Admission does not resolve names. So `localhost` beside `127.0.0.1`, or two different names for
one address, pass admission, and the second bind fails at `Start` (on both probed systems for `localhost`). Also not
promised: listeners opened with `tcp4`/`tcp6`/`udp4`/`udp6` or `SO_REUSEPORT`, other OSes, other Go versions.

## Options considered

| Option | Cost | Verdict |
| --- | --- | --- |
| Do nothing | Start fails loudly on the cases above; admission exists to catch them | rejected by #328 |
| Normalise `ResourceID` text (`http` becomes `tcp`), keep string equality | `registry.go:375` would stop seeing a protocol-label drift between declared and constructed ports; wildcards still pass (`0.0.0.0` beside `::` fails at Start on both OSes) | rejected |
| Text hosts, transport only | the same wildcard gap | rejected |
| Per-OS rule (`runtime.GOOS`) | whether a composition is valid would depend on where it boots; only two systems probed | rejected |
| Resolve names at admission | predicts a resolver answer that may differ at bind time | rejected |
| **Clash relation over `NetworkFacts`, `ResourceID` text unchanged** | about 40 lines; one `netip` import | **recommended** |

## Behaviour

- `CreateComponent` refuses a component one of whose claims clashes with a claim an admitted component holds.
  - The error is of the invalid class (`errs.IsInvalid`) and reads:
    `resource conflict: <claim> already used by component '<holder>' as <held claim>`.
  - Each claim is shown as its `ResourceID` text.
  - When several admitted components clash, the holder named is the first in instance-name order.
  - The admitted set is unchanged.
- `Declare` and `CreateComponent` refuse a component two of whose own claims clash, with:
  `resource conflict: <later claim> already used by another port of component '<name>' as <earlier claim>`.
  Claims are ordered inputs first, then outputs, each in declaration order.
- Today's texts remain a prefix of the new ones. For example:
  `resource conflict: tcp:127.0.0.1:8080 already used by component 'first' as tcp:127.0.0.1:8080`.
- `ResourceID`, `Declaration.ExclusiveResources` (sorted text) and the declared-versus-constructed comparison are
  unchanged. File ports still share.

## Sketch (the developer may differ; the mutants below are written against it)

`component/port_facts.go`, after `NetworkFacts.Port` (`:145`), importing `net/netip`:

```go
// transport is the socket type a listener for this port opens: UDP for the
// protocol label "udp", TCP for every other label (owner ruling, #328).
func (f NetworkFacts) transport() string {
    if f.protocol == "udp" {
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
// address; for a host name it returns the zero Addr.
func hostAddr(host string) netip.Addr {
    addr, err := netip.ParseAddr(host)
    if err != nil {
        return netip.Addr{}
    }
    return addr.Unmap()
}
```

`component/registry.go`, importing `maps`:

- `exclusiveClaims(inputFacts, outputFacts []PortFacts) []PortFacts` returns the `IsExclusive` facts, inputs first.
- `claimsClash(a, b PortFacts) bool` calls `NetworkFacts.clashes` when both have network facts, and otherwise
  compares `ResourceID` text (today's rule, kept so the check stays closed if another kind ever becomes exclusive).
- `exclusiveResources` (`:619`) checks each claim against the earlier ones with `if claimsClash(claim, earlier) {`,
  then returns the sorted `ResourceID` texts.
- `checkResourceConflictsLocked(candidate Declaration)` (`:575`; its call at `:406` passes `prepared`) loops
  `for _, holder := range slices.Sorted(maps.Keys(r.declarations)) {` over each holder's `exclusiveClaims`, with
  `if claimsClash(claim, heldClaim) {`.

## Ruling to implementation

Line numbers are rewritten at the commit that lands task 2.2.

| Ruling | Implemented | Proved |
| --- | --- | --- |
| Only `udp` is UDP; every other label is TCP | `component/port_facts.go`, `NetworkFacts.transport` | `TestRegistryRefusesNetworkClaimsThatCannotBothBind` rows R1-R3; `TestRegistryAdmitsNetworkClaimsThatCanBothBind` rows A1-A2 |
| Hosts decided from the probe: wildcard clashes with any host | `NetworkFacts.clashes`, the `IsUnspecified` branch | R4-R8, R11; A3-A5 show that specific hosts do not clash |
| One address in two spellings clashes | `hostAddr` (`Unmap`) and `a == b` | R9, R10 |
| Names are not resolved (not promised) | `NetworkFacts.clashes`, the text branch | R12 (same text refused), A7 (`localhost` beside `127.0.0.1` admitted) |
| Between components, invalid class, both claims and holder named | `component/registry.go` `checkResourceConflictsLocked` | `TestRegistryRefusesASecondComponentOnATakenAddress`; R rows on the two-component path |
| Within one component, through `Declare` and `CreateComponent` | `component/registry.go` `exclusiveResources` | `TestRegistryRefusesOneComponentClaimingAnAddressTwice`; R rows on the `Declare` path |
| One holder named, deterministically | the sorted holder loop | `TestRegistryNamesTheFirstClashingHolderByName` |

## Tests

All in `component/registry_boot_admission_test.go`. The oracle is the literal text and literal outcome of each row,
taken from the probe table, never computed from `NetworkFacts`.

- `TestRegistryRefusesASecondComponentOnATakenAddress` (`:191`): its expected text becomes
  `resource conflict: tcp:127.0.0.1:8080 already used by component 'first' as tcp:127.0.0.1:8080`.
- `TestRegistryRefusesOneComponentClaimingAnAddressTwice` (`:250`): its expected text becomes
  `resource conflict: tcp:0.0.0.0:8080 already used by another port of component 'gateway' as tcp:0.0.0.0:8080`.
  It gains a row: input `http` and output `websocket` on `0.0.0.0:8080`, expecting
  `resource conflict: websocket:0.0.0.0:8080 already used by another port of component 'gateway' as http:0.0.0.0:8080`.
- `TestRegistryRefusesNetworkClaimsThatCannotBothBind` (new). Each row is two `NetworkPort`s with their
  `ResourceID` literals, run three ways: first then second as two components, second then first, and both as ports
  of one component through `Declare`. Each run checks `errs.IsInvalid` and the exact text.
  - R1: http `0.0.0.0:8080` / websocket `0.0.0.0:8080`
  - R2: http `0.0.0.0:8080` / tcp `0.0.0.0:8080`
  - R3: `UDP` `0.0.0.0:8080` / tcp `0.0.0.0:8080`
  - R4: tcp `0.0.0.0` / tcp `127.0.0.1`
  - R5: udp `0.0.0.0:14550` / udp `127.0.0.1:14550`
  - R6: tcp `0.0.0.0` / tcp `::`
  - R7: tcp `::` / tcp `::1`
  - R8: tcp `0.0.0.0` / tcp `::1`
  - R9: tcp `::ffff:127.0.0.1` / tcp `127.0.0.1`
  - R10: tcp `0:0:0:0:0:0:0:1` / tcp `::1`
  - R11: tcp `0.0.0.0` / tcp `localhost`
  - R12: tcp `localhost` / tcp `localhost`
  - Port 8080 unless shown.
- `TestRegistryAdmitsNetworkClaimsThatCanBothBind` (new, the sensitivity pair). The same three paths; each admits
  both claims.
  - A1: tcp `0.0.0.0` / udp `0.0.0.0`
  - A2: http `0.0.0.0` / udp `0.0.0.0`
  - A3: tcp `127.0.0.1` / tcp `::1`
  - A4: udp `127.0.0.1` / udp `::1`
  - A5: tcp `127.0.0.1` / tcp `127.0.0.2`
  - A6: tcp `0.0.0.0:8080` / tcp `0.0.0.0:8081`
  - A7: tcp `localhost` / tcp `127.0.0.1` (pins the declared "names are not resolved")
- `TestRegistryNamesTheFirstClashingHolderByName` (new): `beta` on tcp `127.0.0.1:8080` and `alpha` on tcp
  `::1:8080` are admitted. `gamma` on tcp `0.0.0.0:8080` is then refused with
  `resource conflict: tcp:0.0.0.0:8080 already used by component 'alpha' as tcp:::1:8080`, and `Snapshots` holds
  `alpha` and `beta`.

These tests bind nothing, so the fixed-port guard does not apply; the existing tests already use 8080 the same way.

### Mutants (each builds; each run with `task mutate:check` on its named test)

| Mutant | Change | Detected by |
| --- | --- | --- |
| M1 | `if f.protocol == "udp" {` becomes `if f.protocol != "tcp" {` | R2 |
| M2 | `if a.IsUnspecified() \|\| b.IsUnspecified() {` becomes `if false && (a.IsUnspecified() \|\| b.IsUnspecified()) {` | R4 |
| M3 | `return addr.Unmap()` becomes `return addr` | R9 |
| M4 | in `checkResourceConflictsLocked`, `if claimsClash(claim, heldClaim) {` becomes `if claim.ResourceID() == heldClaim.ResourceID() {` | R1, two-component path |
| M5 | in `exclusiveResources`, `if claimsClash(claim, earlier) {` becomes `if claim.ResourceID() == earlier.ResourceID() {` | R1, `Declare` path |
| M6 | `for _, holder := range slices.Sorted(maps.Keys(r.declarations)) {` becomes `for holder := range maps.Keys(r.declarations) {` (`slices` is still used at `registry.go:377`) | `TestRegistryNamesTheFirstClashingHolderByName` |

M6's detection depends on map order: it fails about half the time. `mutate:check` runs `-count=1`
(`internal/harness/mutcheck/runs.go:88`), so a single run may survive; that is reported as such, never as detection.
The developer also runs the mutated copy with `go test -count=20 -run TestRegistryNamesTheFirstClashingHolderByName`
and records how many of the 20 runs failed.

**Uncovered, recorded:**

- `claimsClash`'s text fallback: no exclusive kind other than network exists.
- Two different host names for one address: the outcome depends on the resolver, and it is not promised.

### Generated checks: not used

The rule is a symmetric predicate over two claims plus an add-only history. The examples cover it because:

- **Interacting inputs.** The predicate reads three things:
  - transport: same or different, including two TCP labels and the literal `UDP`;
  - port: same or different;
  - host relation: wildcard on either side, one address in two spellings, two distinct addresses, same name, name
    beside an address.

  Each combination that changes the outcome is a row: R1-R12 refuse and A1-A7 admit. Each transport branch, the
  wildcard branch, the address branch and the text branch is proved separately by M1-M3 and A7.
- **Symmetry.** Every row runs in both orders, and within one component as well.
- **History.** The registry only adds components, and a repeated name is refused first
  (`registry_boot_admission_test.go:88`). The only histories are "a holder, then a claimant" (both orders per row)
  and "several holders, then a claimant" (`TestRegistryNamesTheFirstClashingHolderByName`).

There is no input format with size limits: host text is parsed by `netip.ParseAddr` only.

## Who should adopt it

`claimsClash` is the one clash rule. When the pin's `composition/validate.go:139-155` is ported (change 3), it must
call it rather than compare `Declaration.ExclusiveResources` text as map keys. Exposing the rule to `composition` is
that port's design. One tracking issue is filed (task 3.1).

## Docs and ledger

- `component/README.md:208-209` and `:215`: the refusal is for claims that clash, with the rule in one sentence and
  "host names are not resolved".
- `component/port_network.go:7`: the `Protocol` comment says `udp` means UDP and any other label means TCP.
- `docs/admission-ledger.yaml`, `component` row: adaptation (13), in `one-network-address-clash` (#328): "exclusive
  network claims clash by transport (`udp` or TCP), port and host (wildcard, one IP address, or same text), not by
  resource text". `proving_tests` gains the three new tests. No `carry` row changes.

## Merge order

- #329 (config) and #337 (rulepack) share no file; whichever lands second merges `origin/main`.
- #280 item 2 would join `Declaration`'s port and facts lists, which this check reads; whichever lands second adapts
  `exclusiveClaims`'s callers.

## Owner questions

1. Refuse a wildcard plus a specific host on one TCP port everywhere, though macOS binds both? (Recommended: yes.)
2. `UDP` (upper case) is TCP under the ruling's words; R3 pins it. Confirm.
3. The refusal names the holder and both claims, but not the refused component (unchanged from today). Add it?

## Not in scope

- Resolving host names.
- Any non-network exclusive kind.
- `composition`'s check (adoption, above).
