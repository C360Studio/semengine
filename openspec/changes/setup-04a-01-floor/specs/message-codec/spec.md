# message-codec

## ADDED Requirements

### Requirement: Message timestamps travel as integer milliseconds

`BaseMessage.MarshalJSON` SHALL write `meta.created_at` and `meta.received_at` as integer milliseconds since the Unix
epoch, writing the zero time as 0. `Decoder.Decode` SHALL read a timestamp that is absent, `null`, or an integer
literal in `int64`'s range as that many milliseconds, with absent, `null` and 0 each meaning the zero time, and SHALL
refuse any other form: a string, a number with a fraction or an exponent, a boolean, an object, or an integer outside
`int64`'s range (#9 comment 5969522395, item 7). This is changed behaviour: at the pin, decoding read a number up to
10^12 as seconds, so an instant before 2001-09-09 came back wrong.

#### Scenario: An instant before 2001 survives the wire

- **WHEN** a message created at an instant before 2001-09-09, with millisecond precision, is encoded and decoded
- **THEN** the wire holds that instant's Unix milliseconds, and the decoded timestamps equal the instants encoded

#### Scenario: A timestamp that is not an integer

- **WHEN** a message's timestamp is a string, a fraction, an exponent, a boolean, an object or an integer outside
  `int64`'s range
- **THEN** `Decode` returns an error

#### Scenario: No timestamp

- **WHEN** a message's timestamps are absent, `null` or 0
- **THEN** `Decode` accepts it, and both timestamps are the zero time

### Requirement: Every string a message encodes is valid UTF-8

`encoding/json` writes each invalid UTF-8 byte of a string as U+FFFD, which would put a different string on the wire
with no error. `BaseMessage.MarshalJSON` SHALL refuse, with an invalid-data error, a message whose source or any type
component is not valid UTF-8 (#9 comments 5969776736 item 2, 5970334875). `Type.Validate` SHALL refuse a type
component that is not valid UTF-8. `GenericJSONPayload.MarshalJSON` SHALL refuse a string or map key that is not
valid UTF-8 at any depth of `Data`, naming its path. A valid string, U+FFFD itself included, SHALL reach the wire
unchanged.

#### Scenario: Source that is not UTF-8

- **WHEN** a message whose source holds an invalid UTF-8 byte is encoded
- **THEN** encoding returns an invalid-data error, and a message with a valid source, U+FFFD included, encodes and
  decodes unchanged

#### Scenario: Type component that is not UTF-8

- **WHEN** a message whose domain, category or version holds an invalid UTF-8 byte is encoded
- **THEN** encoding returns an invalid-data error

#### Scenario: Generic data holding a string that is not UTF-8

- **WHEN** a generic JSON payload holds a string or a map key that is not valid UTF-8, nested at any depth
- **THEN** encoding returns an error that names the string's path

### Requirement: Generic JSON data is JSON-shaped

`GenericJSONPayload.MarshalJSON` SHALL accept in `Data` only JSON-shaped values: `map[string]any`, `[]any`,
`string`, a Go number (`int`, `int8` to `int64`, `uint`, `uint8` to `uint64`, `uintptr`, `float32`, `float64`),
`json.Number`, `bool` or nil, nested to any depth. It SHALL check each value by its exact type before encoding and
SHALL refuse any other value (a struct, `time.Time`, a pointer, a named type, or a typed map or slice such as
`map[string]string`) with an invalid-data error that names its type and its path, and SHALL refuse a map or list that
contains itself (#9 comments 5972117486, 5972208367).

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

### Requirement: A valid message survives the wire unchanged

A message that validates SHALL survive `json.Marshal` and `Decoder.Decode`, with the payload's type registered,
equal in full: its ID, type, payload, source and both timestamps.

#### Scenario: Round trip

- **WHEN** a message built through `NewBaseMessage` with a valid UTF-8 source and two millisecond timestamps is
  encoded and then decoded
- **THEN** the decoded message equals the one encoded in ID, type, payload, source and both timestamps
