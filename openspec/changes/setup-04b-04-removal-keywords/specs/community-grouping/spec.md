# community-grouping Delta

## ADDED Requirements

### Requirement: Keywords are the group's distinctive words

Keywords SHALL be the `MaxKeywords` words that score best, ties broken lexically, where a word's score is the share
of the group's members holding it times the natural log of the graph's members with words over the graph's members
holding it. A change outside the group SHALL change its keywords only when the group's record is next built.

#### Scenario: A word common to the graph

- **WHEN** every entity of the graph holds one word and a group's members also hold a word no other group holds
- **THEN** the second word is a keyword of the group and the first is not (`TestKeywordsScoredWhenWritten`)

#### Scenario: Known-answer quality

- **WHEN** the scorecard corpus is grouped by each section's parent directory
- **THEN** the share of keywords held by a fifth of all sections, and the share of the first ten hits inside the
  group when its keywords are searched on the word index, each hold the floor `TestKeywordsScorecard` records against
  a plain TF-IDF over all of every section's words computed in the test

#### Scenario: What holding a word means

- **WHEN** a word is outside a member's `MaxEntityTerms` most frequent words, or is held by one member of a group of
  two or more
- **THEN** that member does not hold it, and a word held by one member is no keyword (`TestKeywordsScoredWhenWritten`)

#### Scenario: Only touched groups are written

- **WHEN** an entity outside a group changes and a tick runs
- **THEN** only that entity's groups are written, and the untouched group keeps its keywords
  (`TestKeywordsScoredWhenWritten`, `TestTickCost`)

#### Scenario: Keywords ignore IDs and predicates

- **WHEN** every member shares its predicates and ID positions
- **THEN** no keyword comes from a predicate or ID position (`TestKeywordsIgnoreIDAndPredicates`)

### Requirement: Representatives are the smallest members with text

Representatives SHALL be the lexically smallest members holding text under the component's text rule, found without
a scan of the group.

#### Scenario: A representative is deleted

- **WHEN** the smallest text-bearing member is deleted
- **THEN** the next smallest text-bearing member becomes a representative (`TestRepresentativesWithoutScan`)

## REMOVED Requirements

### Requirement: Keywords and representatives come from the group alone

**Reason**: Keywords are scored against the whole graph (design D8, owner question 1), so a change outside a group
can change its keywords when it is next written. The rest moves to "Keywords are the group's distinctive words" and
"Representatives are the smallest members with text".
