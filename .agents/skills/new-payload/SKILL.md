---
name: new-payload
description: >-
  Checklist for adding a payload type to the payload registry. Use when creating a message type that travels inside a
  BaseMessage, or any type a decoder must rebuild from its registered name.
---

# Add a payload type

Start from the type the user names. A payload is the typed body of a `message.BaseMessage`; the payload registry
(`payloadregistry`) maps each type's name, `domain.category.version`, to a factory, so a decoder can rebuild the
payload from the wire.

## Step 1: Define the type

The struct implements `message.Payload` (`Schema() message.Type`, `Validate() error`, `json.Marshaler`,
`json.Unmarshaler`):

```go
// yourpackage/types.go
package yourpackage

import (
    "fmt"

    "github.com/c360studio/semengine/message"
)

const (
    Domain          = "yourdomain"
    CategoryYourCat = "your_category"
    Version         = "v1"
)

type YourMessage struct {
    ID      string `json:"id"`
    Content string `json:"content"`
}

func (m *YourMessage) Schema() message.Type {
    return message.Type{Domain: Domain, Category: CategoryYourCat, Version: Version}
}

func (m *YourMessage) Validate() error {
    if m.ID == "" {
        return fmt.Errorf("id is required")
    }
    return nil
}
```

If the payload is a graph fact and not only a transport envelope, it also implements `graph.Graphable`
(`EntityID() string`, `Triples() []message.Triple`). `inference.ContainerEntity` (`graph/inference/container_entity.go`)
is a worked example in this tree.

## Step 2: MarshalJSON and UnmarshalJSON

A payload's `MarshalJSON` marshals only its own fields; `BaseMessage` builds the `{"type":...,"payload":...}`
envelope around it (Step 5). Use a type alias, or `json.Marshal` calls the same method again and never returns:

```go
func (m *YourMessage) MarshalJSON() ([]byte, error) {
    type Alias YourMessage
    return json.Marshal((*Alias)(m))
}

func (m *YourMessage) UnmarshalJSON(data []byte) error {
    type Alias YourMessage
    return json.Unmarshal(data, (*Alias)(m))
}
```

## Step 3: Add RegisterPayloads to your package

No `init()`. Export `RegisterPayloads(reg *payloadregistry.Registry) error`, which a composition root calls. The
registration carries what the framework knows about the type: the factory, the indexing profile graph-ingest stamps
on an entity born with the type when the producer declares none, and any projection contracts bound to the type.
graph-ingest refuses an `entity.create` stamp its registry does not hold with `message_type_unregistered`.

```go
// yourpackage/payload_registry.go
package yourpackage

import (
    "github.com/c360studio/semengine/payloadregistry"
    "github.com/c360studio/semengine/pkg/projection/contract"
    "github.com/c360studio/semengine/vocabulary"
)

// RegisterPayloads registers YourMessage (yourdomain.your_category.v1) with reg.
func RegisterPayloads(reg *payloadregistry.Registry) error {
    return reg.Register(&payloadregistry.Registration{
        Domain:      Domain,
        Category:    CategoryYourCat,
        Version:     Version,
        Description: "Description of your message type",
        Factory:     func() any { return &YourMessage{} },
        // content, control, signal or trace; "" is accepted and counted under
        // indexing_profile_default_total{message_type}.
        IndexingProfile: vocabulary.IndexingProfileControl,
        // Optional. An empty contract MessageType is filled with this type;
        // a different one is a registration error.
        Contracts: []contract.Contract{YourBirthContract()},
    })
}
```

`Register` returns an error, never panics, when `Domain`, `Category`, `Version` or `Factory` is empty, when the name
is already registered, when the factory's payload reports a different `Schema()`, when `IndexingProfile` is not one
of the vocabulary's, or when a contract names another type. Join several registrations in one package with
`errors.Join`.
A registered Graphable's `EntityID()` and `Triples()` never panic on decoded input: return `""` or omit the
triple whose identity cannot be constructed, and let graph-ingest reject it.

A unit test that needs only a type name to pass graph-ingest's create check registers a stub with
`payloadfixture.RegisterTestType(t, reg, types.Type{...})` (`internal/harness/payloadfixture`); production code has
no such helper.

## Step 4: Call it from the composition root

A `RegisterPayloads` that nothing calls never runs, and an import does not call it. No package of this module calls
two packages' registrations (`TestNoAggregatorPackage`): the consumer's `main` package, its composition root, calls
each package's `RegisterPayloads` itself. This module has no binary of its own. Its packages that register payloads
today are `message`, `graph/inference` and `pkg/lifecycle`.

A type registered in one of a consumer's binaries but not another fails to decode in the second. Check every binary
(Step 6).

## Step 5: Round-trip through the production decoder

Drive the real wrap (`message.NewBaseMessage`) through the real decoder (`message.NewDecoder`), never a cast to an
anonymous struct:

```go
func TestYourMessage_ProductionDecoderRoundTrip(t *testing.T) {
    reg := payloadfixture.NewForTest(t)
    require.NoError(t, yourpackage.RegisterPayloads(reg))

    msg := &yourpackage.YourMessage{ID: "test-1", Content: "hello"}
    data, err := json.Marshal(message.NewBaseMessage(msg.Schema(), msg, "test"))
    require.NoError(t, err)

    decoded, err := message.NewDecoder(reg).Decode(data)
    require.NoError(t, err)

    got, ok := decoded.Payload().(*yourpackage.YourMessage)
    require.Truef(t, ok, "decoded payload must be *YourMessage, got %T", decoded.Payload())
    require.Equal(t, "test-1", got.ID)
}
```

## Step 6: Trace registration from every composition root

Find each `main` package that should carry the type and follow the call to your `RegisterPayloads` from it. A search
hit on the package name is not proof of a call. Where the call is missing, a message of the type fails to decode
with `unregistered payload type: domain.category.version` (`message/base_message.go`); there is no fallback.

## Checklist

- [ ] The `Schema()` constants match the `Registration`.
- [ ] `MarshalJSON` and `UnmarshalJSON` use a type alias; no `BaseMessage` literal.
- [ ] `RegisterPayloads(reg *payloadregistry.Registry) error` is in the package; no `init()`.
- [ ] The registration declares its `IndexingProfile` and any `Contracts` bound to the type.
- [ ] A Graphable's `EntityID()` and `Triples()` return `""` or omit a triple rather than panic on decoded input.
- [ ] Every type stamped on `entity.create` is registered where graph-ingest runs; unit tests use
      `payloadfixture.RegisterTestType`.
- [ ] The consumer's composition root calls `RegisterPayloads`; no package of this module aggregates it.
- [ ] The production-decoder round-trip test passes.

## Common mistakes

| Symptom | Cause | Fix |
| --- | --- | --- |
| `unregistered payload type: domain.category.version` at decode | The package's `RegisterPayloads` is never called | Call it from the composition root |
| Stack overflow in `Marshal` or `Unmarshal` | No type alias | Add `type Alias YourMessage` |
| `Register` returns a schema error | `Schema()` and the `Registration` disagree | Use the same constants in both |
| `Register` returns "already registered" | The name is registered twice | Pick another name, or remove the second call |
| Decodes in one binary, fails in another | One composition root lacks the call | Trace every root (Step 6) |

## Debugging

```go
// Every registered type in a registry.
for name, r := range reg.List() {
    fmt.Printf("%s: %s\n", name, r.Description)
}

// The wire shape a BaseMessage produces.
data, _ := json.MarshalIndent(message.NewBaseMessage(msg.Schema(), msg, "debug"), "", "  ")
fmt.Println(string(data))
```
