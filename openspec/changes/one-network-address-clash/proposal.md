# Proposal: one-network-address-clash

## Why

The registry refuses a second claim on an exclusive network address by comparing each port's `ResourceID` text,
`protocol:host:port` (`component/registry.go:575-587`, `:619-635`). The protocol is a free label
(`component/port_codec.go:286` only refuses an empty one), so `http:0.0.0.0:8080`, `websocket:0.0.0.0:8080` and
`tcp:0.0.0.0:8080` pass as three addresses although all three listen on one TCP port (#328). Hosts are compared as
text too, so `0.0.0.0:8080` and `::` on the same port pass, though Go binds both to one dual-stack socket and the
second bind fails at `Start` on macOS and Linux (probe, `notes/probe.md`).

The owner ruled on 2026-10-20 (#328):

- Transport: "Only \"udp\" is UDP; every other label is TCP".
- Hosts: "Yes: test both systems and decide it in this PR".

## What Changes

- Two network claims clash when they have the same transport (UDP for the label `udp`, TCP for any other label) and
  the same port, and either host is a wildcard, or both hosts are IP addresses naming one address, or both hosts are
  the same text. Host names are not resolved.
- The registry refuses a clash between components and within one component with this rule, through `Declare` and
  `CreateComponent`, still of the invalid class.
- The refusal names both claims: `resource conflict: websocket:0.0.0.0:8080 already used by component 'first' as
  http:0.0.0.0:8080`. When several admitted components clash, it names the first by instance name.
- `ResourceID` text, `Declaration.ExclusiveResources` and the declared-versus-constructed comparison are unchanged.

Retires #328.

## Capabilities

- `component-registration`: MODIFIED "Admission refuses a second claim on an exclusive resource".

## Impact

- `component/port_facts.go`, `component/registry.go`, `component/registry_boot_admission_test.go`,
  `component/README.md`, `component/port_network.go` (doc comment), `docs/admission-ledger.yaml` (`component` row).
  No exported signature changes.
- A composition accepted today that pairs two protocol labels on one TCP port, a wildcard and another host on one
  port, or one address in two spellings, is refused at admission instead of failing at `Start`. On macOS, a wildcard
  and a specific host on one TCP port, which macOS binds, are refused as well, because Linux refuses them. No consumer
  composition does any of these: semsource and semboids each declare one network port.
