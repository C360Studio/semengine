package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/c360studio/semengine/internal/logforwarderpolicy"
	"github.com/c360studio/semengine/pkg/errs"
)

// logForwarderServiceName is the name RegisterAll gives the log forwarder.
const logForwarderServiceName = "log-forwarder"

// LogForwarderConfig holds configuration for the LogForwarder service. The service forwards
// nothing yet: its values are checked and reported at Start, not applied.
// Note: The service is enabled/disabled via types.ServiceConfig.Enabled at the outer level.
type LogForwarderConfig struct {
	// MinLevel is the minimum level a forwarded record would need (DEBUG, INFO, WARN, ERROR),
	// in any letter case. Default: INFO. Checked and reported, not applied.
	MinLevel string `json:"min_level"`

	// ExcludeSources lists the source prefixes, in dotted notation, whose records would not be
	// forwarded: excluding "a.b" would also exclude "a.b.c" but not "a". Entries are trimmed,
	// empty ones dropped and duplicates removed. Checked and reported, not applied.
	ExcludeSources []string `json:"exclude_sources"`
}

// LogForwarder forwards nothing: no log handler in this module publishes logs to NATS. It checks
// its configuration, runs the base service's lifecycle, and says so in one record at Start.
type LogForwarder struct {
	*BaseService

	policy logforwarderpolicy.Policy
}

// NewLogForwarderService builds the log forwarder from its JSON configuration
// (LogForwarderConfig). A key the configuration does not define, or one in another letter case,
// is refused, naming the key. It refuses, as invalid data and naming the input, nil dependencies
// and a min_level other than DEBUG, INFO, WARN or ERROR. It needs no NATS client and does no I/O.
func NewLogForwarderService(rawConfig json.RawMessage, deps *Dependencies) (Service, error) {
	if deps == nil {
		return nil, refuseLogForwarder(errors.New("log-forwarder requires dependencies"), "check dependencies")
	}
	// The strict decode is a check only: Resolve owns the defaults, normalisation and level check.
	var cfg LogForwarderConfig
	if err := decodeStrictServiceJSON(rawConfig, &cfg); err != nil {
		return nil, refuseLogForwarder(err, "parse log-forwarder config")
	}
	policy, err := logforwarderpolicy.Resolve(rawConfig)
	if err != nil {
		return nil, refuseLogForwarder(err, "validate log-forwarder config")
	}

	logger := slog.Default().With("service", logForwarderServiceName)
	if deps.Logger != nil {
		logger = deps.Logger.With("service", logForwarderServiceName)
	}

	return &LogForwarder{
		BaseService: NewBaseServiceWithOptions(logForwarderServiceName,
			WithLogger(logger), WithMetrics(deps.MetricsRegistry)),
		policy: policy,
	}, nil
}

func refuseLogForwarder(err error, action string) error {
	return errs.WrapInvalid(err, "LogForwarder", "NewLogForwarderService", action)
}

// Start starts the base service, then writes one record at Warn saying the service forwards
// nothing, with the resolved min_level and exclude_sources, and returns nil. It refuses a nil or
// ended context, a second Start and a Start after any Stop (errs.ErrAlreadyStarted); a refused
// Start holds nothing and writes no record. ctx bounds Start only; ending it after Start returned
// stops nothing.
func (lf *LogForwarder) Start(ctx context.Context) error {
	if err := lf.BaseService.Start(ctx); err != nil {
		return err
	}
	lf.logger.Warn("log-forwarder forwards nothing: no log handler in this module publishes logs to NATS",
		"min_level", lf.policy.MinLevel.String(),
		"exclude_sources", lf.policy.ExcludeSources)
	return nil
}
