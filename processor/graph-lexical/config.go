package graphlexical

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/pkg/errs"
)

// defaultMaxTextLen is the body cap when max_text_len is unset: long enough that a code body is
// almost never cut.
const defaultMaxTextLen = 16000

// maxTextLenCeiling is the largest max_text_len accepted, graph-embedding's ceiling: an offloaded
// body is read up to the cap, so an unbounded cap would permit an unbounded read.
const maxTextLenCeiling = 1_000_000

// Config holds the graph-lexical configuration. Decoding is strict: a key it does not define is
// refused, naming it.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// TextSuffixes are the predicate suffixes whose string values are an entity's text, as for
	// graph-embedding; empty means entitytext.DefaultSuffixes.
	TextSuffixes []string `json:"text_suffixes,omitempty" schema:"type:array,description:Predicate suffixes whose values are indexed as text (e.g. .source_code .signature). Defaults to common text predicates,category:advanced"`

	// MaxTextLen is the most characters of an entity's body indexed, as for graph-embedding; the
	// body's tail past it is cut at a word boundary, and the name is never cut. 0 means 16,000.
	MaxTextLen int `json:"max_text_len,omitempty" schema:"type:int,description:Max characters of an entity's body indexed; text beyond is cut at a word boundary. 0 uses the default (16000),min:0,max:1000000,category:advanced"`

	// Dependency startup configuration
	StartupAttempts int `json:"startup_attempts,omitempty" schema:"type:int,description:Max attempts to wait for dependencies at startup,category:advanced"`
	StartupInterval int `json:"startup_interval_ms,omitempty" schema:"type:int,description:Interval between startup attempts in milliseconds,category:advanced"`
}

// Validate implements component.Validatable.
func (c *Config) Validate() error {
	if c.Ports == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "ports configuration required")
	}
	// The index is in memory and answered by the component itself, so a declared output would
	// advertise a write it never performs.
	if len(c.Ports.Outputs) > 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			"graph-lexical declares no output ports; remove ports.outputs")
	}
	if err := validateInputs(c.Ports.Inputs); err != nil {
		return err
	}
	if c.MaxTextLen < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "max_text_len cannot be negative")
	}
	if c.MaxTextLen > maxTextLenCeiling {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("max_text_len exceeds the maximum of %d characters", maxTextLenCeiling))
	}
	if c.StartupAttempts < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "startup_attempts cannot be negative")
	}
	if c.StartupInterval < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "startup_interval_ms cannot be negative")
	}
	return nil
}

// validateInputs accepts exactly one input, the follower's watch of every ENTITY_STATES key with
// no history. Any other input would advertise a feed nothing reads, so it is refused naming the
// port.
func validateInputs(definitions []component.PortDefinition) error {
	if len(definitions) != 1 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("exactly one input port is required, a kv-watch of every %s key; got %d",
				graph.BucketEntityStates, len(definitions)))
	}
	input, err := definitions[0].Resolve(component.DirectionInput)
	if err != nil {
		return errs.WrapInvalid(err, "Config", "Validate", "resolve input port")
	}
	if port, ok := input.Config.(component.KVWatchPort); ok &&
		port.Bucket == graph.BucketEntityStates && len(port.Keys) == 0 && !port.History {
		return nil
	}
	return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
		fmt.Sprintf("input port %q must be a kv-watch of every %s key with no history, the one input graph-lexical reads",
			input.Name, graph.BucketEntityStates))
}

// ApplyDefaults resolves each zero value to the value its reader uses. Validate only checks.
func (c *Config) ApplyDefaults() {
	if len(c.TextSuffixes) == 0 {
		c.TextSuffixes = entitytext.DefaultSuffixes()
	}
	if c.MaxTextLen == 0 {
		c.MaxTextLen = defaultMaxTextLen
	}
	if c.StartupAttempts == 0 {
		c.StartupAttempts = 30 // ~15 seconds with 500ms interval
	}
	if c.StartupInterval == 0 {
		c.StartupInterval = 500 // 500ms
	}
	// A configuration with no ports takes DefaultConfig's; a ports block that is present is used
	// as written, never merged with the defaults.
	if c.Ports == nil {
		c.Ports = DefaultConfig().Ports
	}
}

// DefaultConfig returns a valid default configuration.
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{Name: "entity_watch", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}},
			},
		},
		MaxTextLen: defaultMaxTextLen,
	}
}

// decodeConfig decodes rawConfig strictly: a key Config does not define is refused naming it, and
// data after the object is refused. Empty rawConfig is DefaultConfig.
func decodeConfig(rawConfig json.RawMessage) (Config, error) {
	if len(rawConfig) == 0 {
		return DefaultConfig(), nil
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(rawConfig))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, errs.WrapInvalid(err, "CreateGraphLexical", "factory", "config unmarshal")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Config{}, errs.WrapInvalid(errors.New("data after the configuration object"),
			"CreateGraphLexical", "factory", "config unmarshal")
	}
	return config, nil
}

// schema defines the configuration schema for the graph-lexical component.
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))
