// Package clustering groups entities by entity-ID prefix and publishes one bounded record
// per group to the COMMUNITY_INDEX bucket.
//
// A group (a "community" in the record) is every entity whose ID shares one prefix: level 0
// is org.platform.system.domain, level 1 is org.platform.system and level 2 is
// org.platform (ADR-099, with the levels ADR-102 gives). Nothing is detected: an entity's
// groups follow from its ID alone.
//
// A Maintainer keeps the groups current from entity changes. The caller feeds it one
// ordered watch of ENTITY_STATES: Upsert or Delete for each delivery, with the revision of
// the exact value decoded, then BootstrapDone when the watch's initial replay ends. Tick,
// called on an interval, writes the record of each group that changed since its last
// publication (Community) through a NATSCommunityStorage, and removes nothing until every
// write of that tick has succeeded. GetAllCommunities reads every record back.
//
// The package starts no goroutine and holds no ENTITY_STATES handle; the watch and the
// ticker belong to the caller (processor/graph-clustering). Records are bounded by
// construction (MaxRecordBytes) and hold counts and samples, never a member list. The
// README lists what is not here: label propagation, and the language-model summaries.
package clustering
