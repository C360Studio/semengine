# semteams research rules

Two entity rules from semteams' research pack, which a product loads with its own action type, `publish_agent`, and
predicates it declares itself. `TestUnresolvedValueTemplate` in `matches_test.go` loads each with a handler set holding
a `publish_agent` handler that accepts any body, after declaring the predicates their conditions read, and checks that
an entity built to satisfy them matches through the firing match and through `Matches`. Their condition values hold
the two kinds of `$` a rule pack ships: a regex ending in `$`, which stays as compiled, and a
`$entity.triple.<predicate>.length` template, which is filled from the entity.

The files are copied unchanged from the semteams repository, <https://github.com/C360Studio/semteams>, at commit
`4df44ae877d0c711f769ec6b48b88100dbb267d1`, from `configs/rules/research/`:

| File | What it holds |
| --- | --- |
| `02-plan-to-gather.json` | Fires when the planner decides to gather one to four subtopics; its third condition is a regex over the JSON list |
| `03b-synthesize-when-all-gathers-complete.json` | Fires when as many gatherers have stamped completion as there are subtopics, with a `length_eq` whose value is `$entity.triple.coordinator.decision.subtopics.length` |

semteams is MIT licensed (its `LICENSE` file at that commit, copyright 2025 C360). Do not edit the files: the test
proves that these rules, as the product ships them, load and match.
