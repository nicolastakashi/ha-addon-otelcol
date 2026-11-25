package homeassistantreceiver

import (
	"errors"
	"time"

	"go.opentelemetry.io/collector/config/configopaque"
)

// Config for the Home Assistant receiver.
type Config struct {
	// Endpoint is the Home Assistant WebSocket API URL.
	// Inside an add-on: ws://supervisor/core/websocket.
	Endpoint string `mapstructure:"endpoint"`
	// Token is an admin access token (SUPERVISOR_TOKEN inside an add-on).
	Token configopaque.String `mapstructure:"token"`
	// PollInterval is how often finished automation/script runs are fetched.
	// Home Assistant keeps only the last few runs per item (stored_traces, default 5).
	PollInterval time.Duration `mapstructure:"poll_interval"`
	// Domains whose run traces are collected.
	Domains []string `mapstructure:"domains"`
	// SupervisorEndpoint is the Supervisor REST API, used for container metrics.
	SupervisorEndpoint string `mapstructure:"supervisor_endpoint"`
	// CollectionInterval is how often metrics are read.
	CollectionInterval time.Duration `mapstructure:"collection_interval"`
	// Containers emits the resource usage of Core, the Supervisor and started add-ons
	// (needs the Supervisor manager role).
	Containers bool `mapstructure:"containers"`
	// Entities emits entity states as gauges, with device, area and floor context.
	Entities bool `mapstructure:"entities"`
	// EntityAttributes also emits numeric state attributes (current_temperature,
	// brightness, battery_level...) as homeassistant.<domain>.attribute.<name>.
	EntityAttributes bool `mapstructure:"entity_attributes"`
	// ExcludeEntities are entity_id glob patterns (path.Match) left out of entity metrics.
	ExcludeEntities []string `mapstructure:"exclude_entities"`
	// ExcludeEventTypes are event types not sent as logs (state_changed by default:
	// entity metrics already cover states).
	ExcludeEventTypes []string `mapstructure:"exclude_event_types"`
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	var errs []error
	if c.Endpoint == "" {
		errs = append(errs, errors.New("endpoint must be set"))
	}
	if c.Token == "" {
		errs = append(errs, errors.New("token must be set"))
	}
	if c.PollInterval <= 0 {
		errs = append(errs, errors.New("poll_interval must be positive"))
	}
	if c.CollectionInterval <= 0 {
		errs = append(errs, errors.New("collection_interval must be positive"))
	}
	if len(c.Domains) == 0 {
		errs = append(errs, errors.New("domains must not be empty"))
	}
	return errors.Join(errs...)
}
