// Package inference derives an entity's hierarchy statements from its 6-part
// ID (org.platform.system.domain.type.instance, ADR-102).
//
// HierarchyInference is called by graph-ingest when an entity is born. For
// each enabled level it makes sure the level's container entity exists,
// creating it when it does not, writes the container's inverse edge to the
// new entity through a TripleAdder, and returns the entity's membership
// statement for the caller to write with the entity. With type siblings
// enabled it also returns an edge to each entity of the same type and writes
// the inverse edge on each. Every statement names graph-ingest's hierarchy
// producer (graph.SourceHierarchy) as its source and carries the time of the
// write that triggered it. If any part fails, GetHierarchyTriples returns an
// error and no statements, so the caller can refuse the birth. An entity of
// another deployment, and a container, get no hierarchy.
//
// ContainerEntity is the payload a container entity is born with; its type,
// graph.hierarchy_container.v1, is registered by RegisterPayloads.
package inference
