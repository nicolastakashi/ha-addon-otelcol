// Package homeassistantreceiver turns Home Assistant automation and script
// runs (the trace store behind Settings > Automations > Traces) into OTLP traces, and
// reads container resource usage from the Supervisor as metrics.
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
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Endpoint:     "ws://supervisor/core/websocket",
		PollInterval: 10 * time.Second,
		Domains:      []string{"automation", "script"},

		SupervisorEndpoint: "http://supervisor",
		CollectionInterval: 60 * time.Second,
	}
}

func createTraces(_ context.Context, set receiver.Settings, cfg component.Config, next consumer.Traces) (receiver.Traces, error) {
	return newReceiver(set, cfg.(*Config), next), nil
}

func createMetrics(_ context.Context, set receiver.Settings, cfg component.Config, next consumer.Metrics) (receiver.Metrics, error) {
	return newStatsReceiver(set, cfg.(*Config), next), nil
}
