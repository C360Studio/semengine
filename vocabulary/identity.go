package vocabulary

// CoreIdentityAlias is the framework's identity alias predicate: a value that
// names exactly one entity, which graph-index resolves to that entity's ID.
// Products that write the framework alias use this predicate; it reaches the
// alias index through DiscoverAliasPredicates like any other alias predicate
// (ADR-114 §6).
const CoreIdentityAlias = "core.identity.alias"

func init() {
	Register(CoreIdentityAlias,
		WithDescription("Framework identity alias: a value that resolves to exactly one entity"),
		WithDataType(DataTypeString),
		WithAlias(AliasTypeIdentity, 0))
}
