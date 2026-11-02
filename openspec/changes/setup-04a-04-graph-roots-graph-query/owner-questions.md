# Owner questions: graph-query step 1

Six questions, recommendation first. Detail is in `design.md`.

1. **Keep the wrapper on search replies, and have SemSource change one decode at its cutover. No framework change.**
   Without that change SemSource's search goes empty with no error: no matches, no answer. The wrapper ruling assumed
   consumers already decode by request type, and SemSource's search does not.
2. **Accept that a second answerer on one of our requests is caught only if it declares a port.** SemSource's requests
   declare none, so its old collision would still pass. SemSource's own collision test can close this by comparing
   its requests with our table instead of a hand list. It could declare ports instead. A check at start misses
   anything that starts later, so I would not add one.
3. **Keep the name `kv_revision`; SemConnect changes one line at its cutover.** Until it does, its entity reads fail
   loudly. This one is yours to carry to SemConnect, since we do not edit that repository.
4. **An unknown alias that is not shaped like an ID answers "not found", not "malformed request".** This narrows one
   sentence of the alias decision record, and step 1 amends it.
5. **The GraphQL gateway stays with whoever keeps it; step 1 does not wait.** SemSource still documents `/graphql`, and
   a copied gateway would return two answers nested one level too deep and advertise the old revision name. Does
   SemSource keep it after cutover?
6. **Accept the new names step 1 exports.** The proposal lists them, and the design names who reads each.
