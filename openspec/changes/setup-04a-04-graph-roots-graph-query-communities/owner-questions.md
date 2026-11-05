# Owner questions: setup-04a-04-graph-roots-graph-query-communities

One question. The design is written to the recommended answer.

## 1. A revision check on local search can keep refusing. Accept that for now?

**Yes, accept it. Nobody sends a revision to local search today, and the fix is ready if someone needs one.**

**What happens.** Each community record will say which entity write it last took in. Local search compares a caller's
revision with that number. If the caller wrote to an entity in the group, the number catches up at the next publish
and the search answers. If the caller sends the revision of a write somewhere else, this group's number does not move
until something in the group changes, so the search keeps answering "not reached yet". A quiet small group could do that
for ever. The top-level group takes every write of its deployment and catches up quickly.

**Why it is safe to accept.** No sister project calls local search; only the GraphQL gateway did, and it is not ported.
The weak spot is that a caller learns this only from the docs.

**The options:**

- **Accept (recommended).** The docs say to send the revision of a write to the group you search. Nothing more to
  build.
- **Publish one "caught up to" number after each publish.** No stuck case. It reverses the accepted community ruling's
  "no tick marker is added", and puts a key that is not a community record in the bucket, which storage's read-all
  would then have to skip. About 40 lines in the writer plus the reader (my estimate, not written).
- **Ignore the revision on local search, as on `hierarchyStats`.** Nothing refuses, but a caller who compares the
  reply's number with its own meets the same stuck case, with only a doc to warn them.
