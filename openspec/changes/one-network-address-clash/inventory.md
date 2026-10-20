# Inventory: one-network-address-clash

base: 72570e03db3bae5c828d40bd3197cbf71f2968b9 (origin/main 69436f0 plus the claim commit; no code difference)

Not a porting change: no package is carried from the pin, so the shape sweep and category 6 (pin ADRs per carried
package) do not apply; the pin is read only for who declares network ports.

## 1. The claimed gap

- `component/registry.go:578` — `if resource == existingResource {` (between components: text equality)
- `component/registry.go:628` — `if resources[i] == resources[i-1] {` (within one component: text equality)
- `component/port_network.go:14` — `return fmt.Sprintf("%s:%s:%d", n.Protocol, n.Host, n.Port)`
- `component/port_codec.go:286` — `if strings.TrimSpace(port.Protocol) == "" {` (the only protocol check)
- `component/port_codec.go:289-290` — `if port.Host == "" {` / `port.Host = "0.0.0.0"`
- Searches for an existing transport or address rule: `git grep -n -i "netip\|ParseAddr\|IsUnspecified" -- '*.go'`
  finds only `metric/handler.go:305` — `if addr.IP != nil && !addr.IP.IsUnspecified() {` (the metrics server's own
  listener, not a component port). No transport mapping exists.

## 2. Every spelling of "two claims want one listener"

- `component/registry.go:575-587` `checkResourceConflictsLocked` and `:616-635` `exclusiveResources` (the registry,
  the one home since #324).
- `component/registry.go:69` — `ExclusiveResources []string` (sorted text list; readers below).
- Pin `8b99efe9` `composition/validate.go:58-65` — `if conflict := exclusiveConflict(claimed, declaration); ...` and
  `:139-155` — `holder, taken := claimed[resource]` (text map; `composition` is in change 3, not ported).
- `internal/flowgraph`: its copy was deleted by #327; `git grep -n -i exclusive internal/flowgraph` is empty.

## 3. Adjacent claims

- Spec `openspec/specs/component-registration/spec.md:147` — `### Requirement: Admission refuses a second claim on an
  exclusive resource`.
- Archived `2026-10-19-one-exclusive-address-check` (#324), which filed #328.
- Ledger `docs/admission-ledger.yaml:2251` — `- source_path: component` (`disposition: adapt`, adaptations 1-12).
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`): #332 (this claim, 0 files), #329
  (config, 20 files, none under `component/`, ledger, repository map or this capability), #337 (rulepack, 3 files
  under `openspec/changes/port-rulepack/`). No overlap.
- Open issues: #328 (this). #280 item 2 — "`component.Declaration` keeps ports and their facts in two parallel
  lists": the new check reads `InputFacts`/`OutputFacts`. Searches `component`, `registry`, `exclusive`,
  `network port`, `NetworkPort`, `ResourceID`, `composition`: the remaining hits name the word, not this seam.

## 4. Who it is for

No new exported symbol, field, subject or config key. New unexported: `NetworkFacts.transport`,
`NetworkFacts.clashes`, `hostAddr`, `exclusiveClaims`, `claimsClash`, all called by `exclusiveResources` and
`checkResourceConflictsLocked`.

## 5. Problem shape

Admit-or-refuse at a seam over a symmetric relation. Closest instance:
`component/registry.go:575-587` itself; this design keeps its place and error class and replaces equality with the
relation.

## Readers of the text that stays

- `component/registry.go:375` — `case declaredFacts.ResourceID() != constructedFacts.ResourceID():` (declared versus
  constructed; needs the protocol label, so `ResourceID` must not be normalised).
- `component/registry.go:623` — `resources = append(resources, facts.ResourceID())`.
- `component/registry_boot_admission_test.go:86`, `component/registry_declare_test.go:86` (ExclusiveResources
  literals).

## Network-port declarers

- Pin `input/udp/udp.go:212` — `Config: component.NetworkPort{Protocol: "udp", Host: "0.0.0.0", Port: 14550}` (input).
- Pin `output/websocket/websocket.go:505` — `Config: component.NetworkPort{Protocol: "http", Host: "0.0.0.0", Port:
  port}` (output; a listener).
- semsource `4093d3c` `cmd/semsource/run.go:1034` — `Config: component.NetworkPort{Protocol: "http", Host: wsHost,
  Port: wsPort},` (output listener).
- semboids `37dbdb0` `configs/flock.json:258-261` — `"kind": "network", "protocol": "http", "host": "0.0.0.0",
  "port": 8081` (output listener).
- semconnect `dff1265`, semteams `30ce863a`: `git grep -n "NetworkPort"` empty.
- In repo: `processor/graph-ingest/config_strict_test.go:64` — `"network": component.NetworkPort{Protocol: "tcp",
  Host: "example.test", Port: 443},` (refused as an output graph-ingest never writes).

So output network ports are listeners, and counting them as exclusive (`registry.go:619-623`) is right.

## Adopter seam

1. What a config or component author must know: nothing new. A `NetworkPort` is a listener; its protocol label
   chooses UDP only when it is `udp`.
2. If they do nothing: a composition whose second listener would fail to bind is refused at boot with an
   invalid-class error naming both claims and the holder. On macOS, a wildcard plus a specific host on one TCP port,
   which macOS binds, is refused too.
3. Where they find out: a boot error.
4. What they should have to know: nothing, except that host names are not resolved at admission (declared, not
   promised).
