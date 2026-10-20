# component-registration

## ADDED Requirements

### Requirement: A declaration names its first clash with another

`component.Declaration.ConflictWith(held)` SHALL return nil when no exclusive claim of the declaration clashes with an
exclusive claim of `held`, under the clash rule of "Admission refuses a second claim on an exclusive resource". When
one does, it SHALL return an unwrapped error whose text is `resource conflict: component '<declaration>' claims
<claim>, held by component '<held>' as <held claim>` for the first clash found. The search visits `held`'s claims and,
for each, the declaration's claims, each side inputs first and then outputs in declaration order.
`Registry.CreateComponent` SHALL refuse a clashing candidate, naming the first admitted holder in instance-name order,
with an invalid-class error that wraps this error, so the refusal's text contains it. A declaration SHALL carry no
separate list of exclusive-resource text.

#### Scenario: The method and admission agree

- **WHEN** component `first` is admitted on `http:0.0.0.0:8080` and component `second` declares
  `websocket:0.0.0.0:8080`
- **THEN** `second`'s declaration `ConflictWith` `first`'s returns an error whose text is
  `resource conflict: component 'second' claims websocket:0.0.0.0:8080, held by component 'first' as http:0.0.0.0:8080`,
  and `CreateComponent` refuses `second` with an invalid-class error containing that text
- **AND** when each side declares two claims and only the candidate's second clashes with the holder's first, the
  error names those two claims (`TestDeclarationConflictWithMatchesAdmission`)

#### Scenario: No clash

- **WHEN** the two declarations claim `udp:0.0.0.0:8080` and `tcp:0.0.0.0:8080`
- **THEN** `ConflictWith` returns nil (`TestDeclarationConflictWithAdmitsNonClashingClaims`)
