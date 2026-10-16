# message-codec

## ADDED Requirements

### Requirement: A message type is valid only when Type.Validate accepts it

`types.Type.Validate` SHALL be the one check of a message type's components: each SHALL be non-empty, valid UTF-8,
and free of `.`. `types.Type.IsValid` SHALL report true exactly when `Validate` returns nil.
`BaseMessage.Validate`, and so `BaseMessage.MarshalJSON`, SHALL refuse with an invalid-data error a message whose type
`Validate` refuses.

#### Scenario: A dotted type component

- **WHEN** a message whose category is `a.b` is validated or encoded
- **THEN** `Type.IsValid` reports false, and `BaseMessage.Validate` and `BaseMessage.MarshalJSON` return an
  invalid-data error

#### Scenario: IsValid agrees with Validate

- **WHEN** any type's domain, category or version is empty, not valid UTF-8, or contains `.`
- **THEN** `IsValid` reports false and `Validate` returns an error naming the first such component; otherwise `IsValid`
  reports true and `Validate` returns nil

## MODIFIED Requirements

### Requirement: Generic JSON data is JSON-shaped

`GenericJSONPayload.MarshalJSON` SHALL accept in `Data` only JSON-shaped values: `map[string]any`, `[]any`, `string`, a
Go number (`int`, `int8` to `int64`, `uint`, `uint8` to `uint64`, `uintptr`, `float32`, `float64`), `json.Number`,
`bool` or nil, nested to any depth. It SHALL check each value by its exact type before encoding and SHALL refuse any
other value (a struct, `time.Time`, a pointer, a named type, or a typed map or slice such as `map[string]string`) with
an invalid-data error that names its type and its path, and SHALL refuse a map or list that contains itself (#9 comments
5972117486, 5972208367). `GenericJSONPayload.Validate` SHALL refuse, with an invalid-data error naming the same type and
path, every non-nil `Data` that `MarshalJSON` refuses, and SHALL accept every non-nil `Data` that `MarshalJSON` encodes.

#### Scenario: A value that is not JSON-shaped

- **WHEN** `Data` holds a struct, a pointer, a named type or a typed map or slice at any depth
- **THEN** encoding returns an invalid-data error naming the value's type and path

#### Scenario: A map or list that contains itself

- **WHEN** `Data` holds a map or a list that contains itself
- **THEN** encoding returns an invalid-data error and does not hang

#### Scenario: JSON-shaped values

- **WHEN** `Data` holds only the accepted kinds, nested
- **THEN** encoding gives the standard library's encoding of the same value, and the message decodes through
  `NewDecoder`

#### Scenario: Validate agrees with encoding

- **WHEN** `Validate` is called on a payload whose non-nil `Data` encoding would refuse
- **THEN** `Validate` returns an invalid-data error naming the path encoding would name; and for `Data` that
  encodes, `Validate` returns nil
