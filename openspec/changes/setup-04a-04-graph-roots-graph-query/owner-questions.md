# Owner questions: graph-query step 1

Six questions, recommendation first. Detail is in `design.md`.

1. **Keep the wrapper on search replies; SemSource unwraps once before reading, at its cutover. No framework change.**
   Without that, SemSource's search goes empty with no error: no matches, no answer. The ruling did not know SemSource's
   search ignores an unexpected wrapper instead of failing.
2. **Accept that a second answerer on one of our requests is caught only if it declares a port.** SemSource's graph
   requests declare none, so its old collision would still pass. SemSource's own collision test can close this by
   comparing its requests with our table instead of a hand list. It could declare ports instead. A check at start
   misses anything that starts later, so I would not add one.
3. **Keep the name `kv_revision`; SemConnect makes two small changes at its cutover.** One struct tag, or its entity
   reads fail loudly. And it deletes three config keys the old version ignored, or it refuses to boot. Both are yours to
   carry to SemConnect, since we do not edit that repository.
4. **An unknown alias that is not shaped like an ID answers "not found", not "malformed request".** This narrows one
   sentence of ADR-114, the alias decision record, and step 1 amends it.
5. **The GraphQL gateway stays with whoever keeps it; step 1 does not wait.** Does SemSource keep `/graphql` after
   cutover? SemSource documents it, but our ledger records it as composed and not exercised, and a copy needs the agentic
   and inference packages. By step 5 a copy would nest every wrapped answer one level too deep and still advertise the
   old revision name. Its alias lookup already looks broken: it sends `alias` and the handler reads `aliasOrID` (read,
   not run).
6. **Accept the new names step 1 exports.** In `graph`: `ResponderGraphQuery`, `QueryVerb.Enveloped`, `QueryVerb.Reply`,
   `MinRevisionField`, `ErrorCodeRevisionNotReached`, `RelationshipEdge`. In `composition`: `TypeRequestSubjectConflict`.
   The new `graphquery` package's `Register`, `DeclarePorts`, `CreateGraphQuery`, `DefaultConfig`, `Config`,
   `Component` are the old version's names. The first group's readers are named in the design.
