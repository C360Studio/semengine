# graph-embedding Delta

## ADDED Requirements

### Requirement: An embedder that hangs frees the Worker's lane

The Worker SHALL give each embedder call a deadline 30 seconds after it begins. A call that ends at the deadline SHALL
fail the record `timeout`, and the lane SHALL go on to its next record.

#### Scenario: One hung call on a lane

- **WHEN** a lane holds two pending records, the embedder blocks the first call until its context ends, then serves
  the second
- **THEN** the first record is failed `timeout` and the second is generated, 30 seconds later
  (`TestHungEmbedderFreesWorkerLane`, under `testing/synctest`)
