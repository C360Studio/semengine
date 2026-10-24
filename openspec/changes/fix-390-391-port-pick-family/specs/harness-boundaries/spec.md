# harness-boundaries

## ADDED Requirements

### Requirement: A port-0 pick on every address in test code says why

Every `net.Listen` call in a `*_test.go` file or in any Go file under `internal/harness/` whose network is the literal
`"tcp"` and whose address is the literal `""` or a literal with an empty, `0.0.0.0` or `[::]` host and an empty or
`0` port SHALL have, on the line above it, a comment beginning `// Every address:` followed by a reason.

#### Scenario: Every-address pick without a reason

- **WHEN** a test file calls `net.Listen("tcp", ":0")` with no `// Every address:` comment on the line above
- **THEN** the contract test fails naming the file and line

#### Scenario: Pick in the bound family

- **WHEN** a test file calls `net.Listen("tcp4", ":0")` or `net.Listen("tcp", "127.0.0.1:0")`
- **THEN** the contract test reports nothing for it

#### Scenario: Held port with a reason

- **WHEN** the line above reads `// Every address: holds the port so the server's ":port" bind fails.`
- **THEN** the contract test reports nothing for it

### Requirement: The every-address pick check names the fix and can fail

The every-address pick check's failure SHALL name the file and line and tell the author to pick in the family that is
bound (`tcp4` for Docker, `127.0.0.1:0` or the listener itself for a test server), and a paired sensitivity test SHALL
show it fails on a planted call without a reason. It reads code before a line comment; an address in a variable, a
raw string, a call split over lines, `ListenConfig.Listen`, a production listener given port 0, and whether the
reason holds are review only.

#### Scenario: Planted call in a fake tree

- **WHEN** the sensitivity test plants `net.Listen("tcp", "0.0.0.0:0")` in a test file of a fake tree
- **THEN** the check reports a violation naming that file and line and the family to pick
