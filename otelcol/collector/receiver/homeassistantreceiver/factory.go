// Package homeassistantreceiver turns Home Assistant automation and script
// runs (the trace store behind Settings > Automations > Traces) into OTLP traces, and
// reads container and entity metrics, and sends bus events as logs.
package homeassistantreceiver

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
)

var componentType = component.MustNewType("homeassistant")

// NewFactory creates the receiver factory.
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		componentType,
		createDefaultConfig,
		receiver.WithTraces(createTraces, component.StabilityLevelDevelopment),
		receiver.WithMetrics(createMetrics, component.StabilityLevelDevelopment),
		receiver.WithLogs(createLogs, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Endpoint:     "ws://supervisor/core/websocket",
		PollInterval: 10 * time.Second,
		Domains:      []string{"automation", "script"},

		SupervisorEndpoint: "http://supervisor",
		CollectionInterval: 60 * time.Second,
		ExcludeEventTypes:  []string{"state_changed"},
		ExcludeAttributes:  DefaultExcludeAttributes,
	}
}

func createTraces(_ context.Context, set receiver.Settings, cfg component.Config, next consumer.Traces) (receiver.Traces, error) {
	return newReceiver(set, cfg.(*Config), next), nil
}

func createMetrics(_ context.Context, set receiver.Settings, cfg component.Config, next consumer.Metrics) (receiver.Metrics, error) {
	return newStatsReceiver(set, cfg.(*Config), next), nil
}

func createLogs(_ context.Context, set receiver.Settings, cfg component.Config, next consumer.Logs) (receiver.Logs, error) {
	return newEventsReceiver(set, cfg.(*Config), next), nil
}

// DefaultExcludeAttributes leaves out static configuration and GPS positions (privacy).
var DefaultExcludeAttributes = []string{
	"supported_features", "min", "max", "min_*", "max_*", "*step", "current", "display_precision",
	"latitude", "longitude", "altitude", "gps_accuracy", "radius",
}
