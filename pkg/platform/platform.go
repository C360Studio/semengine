// Package platform holds the platform-identity struct, Config. Its reader is
// package config, whose exported Config names it. No message or vocabulary
// symbol carries platform identity: a deployment's org and platform reach an
// entity only as positions 1-2 of its ID (ADR-102 and ADR-104, docs/adr/).
//
// The struct lives in this leaf package, not in package config, to avoid a
// transitive import cycle: a message package that labels messages with
// platform identity would import config, which would force config to stay
// free of imports from packages like component — even though config
// legitimately needs to consume component port definitions for stream
// derivation. A leaf package breaks the cycle without spreading platform
// identity across multiple definitions.
package platform

// Config defines platform identity and capabilities.
//
// Accessed as platform.Config to match the convention used by sibling
// leaf packages (pkg/security/config.go: security.Config). The
// config-package alias config.PlatformConfig serves package config's own
// call sites.
//
// Six-part federated entity IDs are anchored on Org and ID: they are
// positions 1-2 of every identity this deployment mints
// (org.platform.system.domain.type.instance, ADR-102), so ID is the
// minting deployment authority and nothing else names it. The struct
// is JSON-shaped so it round-trips cleanly through config files.
type Config struct {
	Org          string   `json:"org"`                    // Organization namespace (e.g., "c360", "noaa")
	ID           string   `json:"id"`                     // Platform identifier (e.g., "platform1")
	Type         string   `json:"type"`                   // vessel, shore, buoy, satellite
	Region       string   `json:"region,omitempty"`       // gulf_mexico, atlantic, pacific
	Capabilities []string `json:"capabilities,omitempty"` // radar, ctd, deployment, etc.

	// Environment is a startup log label ("prod", "dev", "test") and nothing
	// else: it separates no deployments. Two deployments declaring the same
	// Org and ID share one configuration bucket and one authority whatever
	// their Environment; give each its own ID ("myapp-dev", "myapp-prod")
	// instead: the bucket name is the separation.
	Environment string `json:"environment,omitempty"`
}
