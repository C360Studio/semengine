# Owner questions: setup-04a-04-graph-roots-graph-query-reads

## 1. Should the graph overview and hierarchy counts be exact?

**Not fully, not yet. Take two small changes, already in the design, and build exact counts when someone needs them.**

**Why not now.** No sister project's production code calls either request. The only caller is the GraphQL gateway, which
is not being ported, and SemSource cut its tool. How large real groups get has not been measured.

**The limit.** Each answer reads one page: the alphabetically first 1,000 entities, about 450 at 2 KB. Past that, type
and child counts are a sample, and the answer says so.

**The options:**

- **As you accepted:** a sample that says it is one. Nothing more to build.
- **A true total (in the design):** "1,000 read of 1,000,000", with no extra scan. About 10 lines plus tests (reviewer's
  estimate, not run), in graph-ingest, a merged package; every prefix reply gains a key.
- **A prefix on the overview (in the design):** one group's types for one group's scan, a few lines. Predicate counts
  cannot be narrowed to a group, so a caller using a prefix must leave predicates out; asking for both is refused.
- **A new count request:** every count exact. About 250 lines (my estimate), in a merged package, for no caller today.

If you want the new request now, or neither small change, say so. The design adjusts either way.

## 2. What should `hierarchyStats` say about how current it is?

**Return it without the wrapper, as direct lookups do.**

Your accepted ruling lists it as wrapped, but also says direct reads from graph-ingest are unwrapped, and this one reads
only graph-ingest. Its answer is current. The wrapper's revision field has nothing true to report.

- **Keep the wrapper, revision 0.** Callers are told to compare that field, so one that does would never see it catch up,
  and a retry loop would spin for ever. Only a doc warns them.
- **Remove the wrapper (recommended).** There is no field to misread. It departs from the ruling's list, which only you
  can accept. The design is written the first way until you say. The change: one table mark, the reply, one test, two
  spec scenarios and a migration line.

Either way `summary` keeps the wrapper. When a caller leaves predicates out, it reports 0, so the same never-clearing
alarm stays there, with only a doc to warn them.
