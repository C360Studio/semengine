# Owner questions: setup-04a-04-graph-roots-graph-query-reads

## 1. Should the graph overview and hierarchy counts be exact?

**Not fully, not yet. Take two small changes, already in the design, and build exact counts when someone needs them.**

**Why not now.** No sister project's production code calls either request. The only caller is the GraphQL gateway, which
is not being ported, and SemSource cut its tool. How large real groups get has not been measured.

**The limit.** Each answer reads one page: the alphabetically first 1,000 entities, about 450 at 2 KB. Past that, type
and child counts are a sample, and the answer says so.

| Option | What you get | Cost |
| --- | --- | --- |
| As you accepted | A sample that says it is one | Nothing more |
| **Add a true total** (in the design) | "1,000 read of 1,000,000" | About 10 lines plus tests in graph-ingest (reviewer's estimate, not run); no extra scan |
| **Add a prefix to the overview** (in the design) | A large graph can show any group, not only its first | A few lines; one group's scan |
| New count request | Every count exact | About 250 lines (my estimate), in a merged package, for no caller today |

If you want the new request now, or neither small change, say so. The design adjusts either way.

## 2. What should `hierarchyStats` say about how current it is?

**Return it without the wrapper, as direct lookups do.**

Your accepted ruling lists it as wrapped, but also says direct reads from graph-ingest are unwrapped, and this one reads
only graph-ingest. Its answer is current. The wrapper's revision field has nothing true to report.

- **Keep the wrapper, revision 0.** Callers are told to compare that field, so one that does would never see it catch up,
  and a retry loop would spin for ever. Only a doc warns them.
- **Remove the wrapper (recommended).** There is no field to misread. It departs from the ruling's list, which only you
  can accept. The design is written the first way until you say; the change is one table mark, one test, one spec clause.

Either way `summary` keeps the wrapper, and reports 0 only when a caller asks to leave predicates out.
