// Package graphclustering is the graph-clustering component: it follows ENTITY_STATES and keeps
// COMMUNITY_INDEX current, one bounded record per entity-ID prefix group, through the
// graph/clustering Maintainer, and reports on the graph-clustering readiness key. A group is
// every entity that shares the first 4, 3 or 2 parts of its ID. At tier 0 nothing reaches an
// external provider: communities are derived from IDs, not detected (ADR-099).
//
// Every delivered change is applied to the Maintainer in delivery order. A ticker publishes the
// changed groups every detection_interval, and only after the follower has applied the whole
// replay and while its feed is intact.
//
// A composition registers the factory with Register and creates the component through the
// component manager, with a configuration that Config describes.
package graphclustering
