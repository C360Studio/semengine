package graphembedding

import (
	"fmt"
	"reflect"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/pkg/errs"
)

// embedderBM25 is the one embedder at tier 0: in-process, no provider.
const embedderBM25 = "bm25"

// defaultWorkers matches embedding.NewWorker's own default, so an unset workers behaves as
// the Worker intends.
const defaultWorkers = 5

// maxSourceTextLenBM25 is the text cap for BM25 when max_text_len is unset. The Worker's own
// default, 8,000, is the cap for other embedders, so the component always passes this value
// instead.
const maxSourceTextLenBM25 = 4000

// Config holds configuration for graph-embedding component. Decoding is strict: a key it
// does not define, batch_size among them, is refused.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	// EmbedderType is the embedder; at tier 0 only bm25 is accepted, and empty means bm25.
	EmbedderType string `json:"embedder_type" schema:"type:string,description:Embedder type (bm25 only at tier 0),category:basic"`

	// Workers is the number of concurrent embedding worker goroutines; 0 means 5.
	Workers int `json:"workers,omitempty" schema:"type:int,description:Number of concurrent embedding worker goroutines (0 = default 5),category:advanced"`

	// MaxTextLen is the source-text cap, in characters, applied before embedding; text past it
	// is truncated at a word boundary. It is part of the dedup key, so changing it re-embeds
	// the entities it affects. 0 means BM25's 4,000.
	MaxTextLen int `json:"max_text_len,omitempty" schema:"type:int,description:Max characters of source text embedded per entity; text beyond is truncated at a word boundary. 0 uses the default (4000),min:0,max:1000000,category:advanced"`

	// TextSuffixes are the predicate suffixes whose string values are embedded; empty means
	// entitytext.DefaultSuffixes.
	TextSuffixes []string `json:"text_suffixes,omitempty" schema:"type:array,description:Predicate suffixes to extract for embedding (e.g. .source_code .signature). Defaults to common text predicates,category:advanced"`

	// Dependency startup configuration
	StartupAttempts int `json:"startup_attempts,omitempty" schema:"type:int,description:Max attempts to wait for dependencies at startup,category:advanced"`
	StartupInterval int `json:"startup_interval_ms,omitempty" schema:"type:int,description:Interval between startup attempts in milliseconds,category:advanced"`
	CoalesceMs      int `json:"coalesce_ms,omitempty" schema:"type:int,description:Debounce window for entity updates in ms. 0=immediate processing,category:advanced"`
}

// Validate implements component.Validatable interface
func (c *Config) Validate() error {
	if c.Ports == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "ports configuration required")
	}
	if len(c.Ports.Inputs) == 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "at least one input port required")
	}
	// The component writes EMBEDDING_INDEX and EMBEDDING_DEDUP itself, not through declared
	// output ports, so a declared output would advertise a write it never performs.
	if len(c.Ports.Outputs) > 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			"graph-embedding declares no output ports; remove ports.outputs")
	}
	if err := validateInputs(c.Ports.Inputs); err != nil {
		return err
	}

	if c.EmbedderType != embedderBM25 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("embedder_type %q is not available at tier 0; the only embedder is %q", c.EmbedderType, embedderBM25))
	}
	if c.Workers < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "workers cannot be negative")
	}
	if c.MaxTextLen < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "max_text_len cannot be negative")
	}
	// An unbounded cap would overflow the offloaded lane's byte budget and permit a huge read.
	if c.MaxTextLen > embedding.MaxSourceTextLenCeiling {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("max_text_len exceeds the maximum of %d characters", embedding.MaxSourceTextLenCeiling))
	}
	return nil
}

// validateInputs accepts the inputs graph-embedding reads: the follower's watch of every
// ENTITY_STATES key with no history, required, and store reads for offloaded bodies. Any
// other input would advertise a feed nothing reads, so it is refused naming the port, as
// graph-index refuses one.
func validateInputs(definitions []component.PortDefinition) error {
	watched := false
	for _, definition := range definitions {
		input, err := definition.Resolve(component.DirectionInput)
		if err != nil {
			return errs.WrapInvalid(err, "Config", "Validate", "resolve input port")
		}
		switch port := input.Config.(type) {
		case component.KVWatchPort:
			if port.Bucket == graph.BucketEntityStates && len(port.Keys) == 0 && !port.History {
				watched = true
				continue
			}
		case component.StoreReadPort:
			continue
		}
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("input port %q must be a kv-watch of every %s key with no history or a store read, the inputs graph-embedding reads",
				input.Name, graph.BucketEntityStates))
	}
	if !watched {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("an input port watching every %s key is required", graph.BucketEntityStates))
	}
	return nil
}

// ApplyDefaults sets default values for configuration, resolving each zero value to the
// value its reader uses. Validate only checks.
func (c *Config) ApplyDefaults() {
	if c.EmbedderType == "" {
		c.EmbedderType = embedderBM25
	}
	if c.Workers == 0 {
		c.Workers = defaultWorkers
	}
	if c.MaxTextLen == 0 {
		c.MaxTextLen = maxSourceTextLenBM25
	}
	if len(c.TextSuffixes) == 0 {
		c.TextSuffixes = entitytext.DefaultSuffixes()
	}
	if c.StartupAttempts == 0 {
		c.StartupAttempts = 30 // ~15 seconds with 500ms interval
	}
	if c.StartupInterval == 0 {
		c.StartupInterval = 500 // 500ms
	}
	// A configuration with no ports takes DefaultConfig's; a ports block that is present is
	// used as written, never merged with the defaults.
	if c.Ports == nil {
		c.Ports = DefaultConfig().Ports
	}
}

// DefaultConfig returns a valid default configuration
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{Name: "entity_watch", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}},
				{Name: "content_store", Config: component.StoreReadPort{Bucket: "MESSAGES"}},
			},
		},
		EmbedderType: embedderBM25,
		Workers:      defaultWorkers,
	}
}

// schema defines the configuration schema for graph-embedding component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))
