# message-codec Specification

## Purpose
The message codec is how `message` turns a message into JSON and back. It guarantees that a valid message comes
back from the wire equal to the one sent: timestamps travel as whole milliseconds, a string that is not valid UTF-8
is refused rather than silently altered, and generic JSON payloads hold only plain JSON values and keep every number
exactly as written.

## Requirements

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

### Requirement: Generic JSON is a fallback that keeps every number

`GenericJSONPayload` (type `core.json.v1`) SHALL be the fallback for JSON whose shape is not known when the code is
written, such as outside input. Code that builds a shape it knows registers a payload type for that shape instead.
Nothing SHALL register `core.json.v1` implicitly: a process that decodes it calls `RegisterPayloads` on the registry its
`Decoder` uses. `GenericJSONPayload.UnmarshalJSON` SHALL accept a body that is a JSON object or `null`, leaving `Data`
nil on a fresh (zero-value) receiver when `data` is `null` or absent (`{"data":null}`, `{}`, `null`; a receiver that
already holds `Data` keeps it for `{}` and `null`, and `{"data":null}` clears it). It SHALL refuse with an invalid-data
error malformed JSON, non-whitespace bytes after the JSON value, a body that is neither an object nor `null` (such as
`[]` or `1`), and a `data` that is neither an object nor `null`. It SHALL keep every number as a `json.Number` holding
its exact literal, so an integer beyond 2^53 and a literal beyond `float64`'s range such as `1e400` keep their exact
value (changed behaviour: at the pin every decoded number became a `float64`; PR #48, Codex F30, F41). A payload built
in process SHALL hold the Go numbers its caller supplied.

#### Scenario: A large integer survives the wire

- **WHEN** a generic JSON payload holding `int64(9007199254740993)`, the largest `uint64` or
  `json.Number("9007199254740993")` is encoded and decoded through `NewDecoder`
- **THEN** the decoded payload holds that number as a `json.Number` with the same literal, and re-encodes to the same
  bytes

#### Scenario: An in-process payload

- **WHEN** a generic JSON payload is built in process from Go numbers
- **THEN** `Data` holds those Go numbers as supplied, until the payload is encoded and decoded

#### Scenario: Decoding without registration

- **WHEN** a `core.json.v1` message is decoded by a `Decoder` whose registry did not get `RegisterPayloads`
- **THEN** `Decode` returns an error

### Requirement: A valid message survives the wire unchanged

A message that validates SHALL survive `json.Marshal` and `Decoder.Decode`, with the payload's type registered,
equal in full: its ID, type, payload, source and both timestamps.

#### Scenario: Round trip

- **WHEN** a message built through `NewBaseMessage` with a valid UTF-8 source and two millisecond timestamps is
  encoded and then decoded
- **THEN** the decoded message equals the one encoded in ID, type, payload, source and both timestamps
