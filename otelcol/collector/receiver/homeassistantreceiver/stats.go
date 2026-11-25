package homeassistantreceiver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
)

// containerStats is the Supervisor stats payload (/core/stats, /supervisor/stats,
// /addons/<slug>/stats). cpu_percent is relative to one CPU (can exceed 100).
type containerStats struct {
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryUsage int64   `json:"memory_usage"`
	NetworkRx   int64   `json:"network_rx"`
	NetworkTx   int64   `json:"network_tx"`
	BlkRead     int64   `json:"blk_read"`
	BlkWrite    int64   `json:"blk_write"`
}

// statsReceiver polls the Supervisor for the resource usage of Home Assistant Core, the
// Supervisor and every started add-on, as semconv container.* metrics labelled with the
// same container.name the journal uses.
type statsReceiver struct {
	cfg    *Config
	logger *zap.Logger
	next   consumer.Metrics
	http   *http.Client
	start  pcommon.Timestamp

	cancel context.CancelFunc
	done   chan struct{}
}

func newStatsReceiver(set receiver.Settings, cfg *Config, next consumer.Metrics) *statsReceiver {
	return &statsReceiver{cfg: cfg, logger: set.Logger, next: next, http: &http.Client{Timeout: 10 * time.Second}}
}

func (r *statsReceiver) Start(_ context.Context, _ component.Host) error {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel, r.done = cancel, make(chan struct{})
	r.start = pcommon.NewTimestampFromTime(time.Now())
	go func() {
		defer close(r.done)
		t := time.NewTicker(r.cfg.CollectionInterval)
		defer t.Stop()
		for {
			if md, err := r.scrape(ctx); err != nil {
				r.logger.Warn("Reading container stats from the Supervisor failed", zap.Error(err))
			} else if err := r.next.ConsumeMetrics(ctx, md); err != nil {
				r.logger.Warn("Dropping container stats", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return nil
}

func (r *statsReceiver) Shutdown(context.Context) error {
	if r.cancel != nil {
		r.cancel()
		<-r.done
	}
	return nil
}

// get decodes the "data" of a Supervisor API response.
func (r *statsReceiver) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.cfg.SupervisorEndpoint+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+string(r.cfg.Token))
	resp, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(&struct {
		Data any `json:"data"`
	}{out})
}

func (r *statsReceiver) scrape(ctx context.Context) (pmetric.Metrics, error) {
	targets := []struct{ name, path string }{
		{"homeassistant", "/core/stats"},
		{"hassio_supervisor", "/supervisor/stats"},
	}
	var addons struct {
		Addons []struct {
			Slug  string `json:"slug"`
			State string `json:"state"`
		} `json:"addons"`
	}
	if err := r.get(ctx, "/addons", &addons); err != nil {
		return pmetric.Metrics{}, err
	}
	for _, a := range addons.Addons {
		if a.State == "started" {
			targets = append(targets, struct{ name, path string }{"addon_" + a.Slug, "/addons/" + a.Slug + "/stats"})
		}
	}

	md := pmetric.NewMetrics()
	sm := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("github.com/nicolastakashi/ha-addon-otelcol/homeassistantreceiver")
	ms := sm.Metrics()
	cpu := gauge(ms, "container.cpu.usage", "{cpu}", "CPU used by the container, in CPUs.")
	// ponytail: semconv calls container.memory.usage a counter; memory goes down, so a gauge.
	mem := gauge(ms, "container.memory.usage", "By", "Memory used by the container.")
	disk := counter(ms, "container.disk.io", "By", "Disk bytes read and written by the container.")
	net := counter(ms, "container.network.io", "By", "Network bytes received and sent by the container.")

	now := pcommon.NewTimestampFromTime(time.Now())
	for _, t := range targets {
		var s containerStats
		if err := r.get(ctx, t.path, &s); err != nil {
			r.logger.Debug("Skipping container stats", zap.String("container", t.name), zap.Error(err))
			continue
		}
		point(cpu.Gauge().DataPoints(), now, 0, t.name).SetDoubleValue(s.CPUPercent / 100)
		point(mem.Gauge().DataPoints(), now, 0, t.name).SetIntValue(s.MemoryUsage)
		dir(point(disk.Sum().DataPoints(), now, r.start, t.name), "disk.io.direction", "read").SetIntValue(s.BlkRead)
		dir(point(disk.Sum().DataPoints(), now, r.start, t.name), "disk.io.direction", "write").SetIntValue(s.BlkWrite)
		dir(point(net.Sum().DataPoints(), now, r.start, t.name), "network.io.direction", "receive").SetIntValue(s.NetworkRx)
		dir(point(net.Sum().DataPoints(), now, r.start, t.name), "network.io.direction", "transmit").SetIntValue(s.NetworkTx)
	}
	return md, nil
}

func gauge(ms pmetric.MetricSlice, name, unit, desc string) pmetric.Metric {
	m := ms.AppendEmpty()
	m.SetName(name)
	m.SetUnit(unit)
	m.SetDescription(desc)
	m.SetEmptyGauge()
	return m
}

func counter(ms pmetric.MetricSlice, name, unit, desc string) pmetric.Metric {
	m := ms.AppendEmpty()
	m.SetName(name)
	m.SetUnit(unit)
	m.SetDescription(desc)
	s := m.SetEmptySum()
	s.SetIsMonotonic(true)
	s.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	return m
}

func point(dps pmetric.NumberDataPointSlice, now, start pcommon.Timestamp, container string) pmetric.NumberDataPoint {
	dp := dps.AppendEmpty()
	dp.SetTimestamp(now)
	if start != 0 {
		dp.SetStartTimestamp(start)
	}
	dp.Attributes().PutStr("container.name", container)
	return dp
}

func dir(dp pmetric.NumberDataPoint, key, value string) pmetric.NumberDataPoint {
	dp.Attributes().PutStr(key, value)
	return dp
}
