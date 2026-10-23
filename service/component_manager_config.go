package service

// ComponentManagerConfig configures the ComponentManager service. It defines no
// key: the manager composes the one boot component set from the configuration
// manager's components, so a strict decode refuses every key, naming it, and the
// retired enabled_components and watch_config fail loudly instead of being ignored.
type ComponentManagerConfig struct{}
