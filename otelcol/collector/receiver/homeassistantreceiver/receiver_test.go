package homeassistantreceiver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

// A real trace/get payload shape (trimmed) from Home Assistant.
const sampleTrace = `{
  "domain": "automation", "item_id": "1791864237116", "run_id": "r2",
  "state": "stopped", "script_execution": "finished",
  "trigger": "time pattern", "last_step": "action/2",
  "timestamp": {"start": "2026-11-24T21:05:00.100000+00:00", "finish": "2026-11-24T21:05:00.400000+00:00"},
  "context": {"id": "01KAV3RQ5Y2G3H4J5K6M7N8P9Q", "parent_id": null, "user_id": null},
  "config": {
    "alias": "Cilindro gestao inteligente",
    "triggers": [{"trigger": "time_pattern", "minutes": "/5"}],
    "actions": [
      {"alias": "Ligar resistencia", "action": "switch.turn_on"},
      {"if": [{"condition": "state", "entity_id": "input_boolean.x", "state": "on"}], "then": []},
      {"action": "script.x"}
    ]
  },
  "trace": {
    "trigger/0": [{"path": "trigger/0", "timestamp": "2026-11-24T21:05:00.100000+00:00"}],
    "action/0": [{"path": "action/0", "timestamp": "2026-11-24T21:05:00.150000+00:00"}],
    "action/1": [{"path": "action/1", "timestamp": "2026-11-24T21:05:00.180000+00:00"}],
    "action/1/if": [{"path": "action/1/if", "timestamp": "2026-11-24T21:05:00.190000+00:00", "result": {"result": false, "params": {"x": 1}}}],
    "action/1/if/condition/0": [{"path": "action/1/if/condition/0", "timestamp": "2026-11-24T21:05:00.200000+00:00", "error": "boom"}],
    "action/2": [{"path": "action/2", "timestamp": "2026-11-24T21:05:00.300000+00:00",
                  "child_id": {"domain": "script", "item_id": "x", "run_id": "c1"}}]
  }
}`

func TestToTraces(t *testing.T) {
	var d traceDetail
	if err := json.Unmarshal([]byte(sampleTrace), &d); err != nil {
		t.Fatal(err)
	}
	spans := toTraces(d).ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	if spans.Len() != 7 {
		t.Fatalf("want root + 6 steps, got %d", spans.Len())
	}
	root := spans.At(0)
	if root.Name() != "automation Cilindro gestao inteligente" {
		t.Errorf("root name %q", root.Name())
	}
	if got := root.EndTimestamp().AsTime().Sub(root.StartTimestamp().AsTime()); got != 300*time.Millisecond {
		t.Errorf("root duration %v", got)
	}
	want, _ := ulidBytes("01KAV3RQ5Y2G3H4J5K6M7N8P9Q")
	if root.TraceID() != want {
		t.Errorf("trace id not derived from the context ULID")
	}
	// name, parent index, end (ms after 21:05:00)
	steps := []struct {
		name   string
		parent int
		endMs  int
	}{
		{"trigger 0: time_pattern", 0, 150},
		{"action 0: Ligar resistencia", 0, 180},
		{"action 1: if", 0, 300},
		{"if", 3, 300},
		{"condition 0: state", 4, 300},
		{"action 2: script.x", 0, 400},
	}
	base := time.Date(2026, 11, 24, 21, 5, 0, 0, time.UTC)
	for i, w := range steps {
		s := spans.At(i + 1)
		if s.Name() != w.name {
			t.Errorf("step %d name %q, want %q", i+1, s.Name(), w.name)
		}
		if s.ParentSpanID() != spans.At(w.parent).SpanID() || s.TraceID() != root.TraceID() {
			t.Errorf("step %q: wrong parent", w.name)
		}
		if e := s.EndTimestamp().AsTime(); !e.Equal(base.Add(time.Duration(w.endMs) * time.Millisecond)) {
			t.Errorf("step %q ends at %v", w.name, e)
		}
	}
	if spans.At(5).Status().Code() != ptrace.StatusCodeError || spans.At(5).Status().Message() != "boom" {
		t.Errorf("step error not mapped")
	}
	if v, ok := spans.At(4).Attributes().Get("homeassistant.step.result.result"); !ok || v.Bool() {
		t.Error("if step: want homeassistant.step.result.result=false")
	}
	if _, ok := spans.At(4).Attributes().Get("homeassistant.step.result.params"); ok {
		t.Error("nested result values must be skipped")
	}
	if v, _ := root.Attributes().Get("homeassistant.last_step"); v.Str() != "action/2" {
		t.Errorf("last_step %q", v.Str())
	}
	if v, _ := spans.At(6).Attributes().Get("homeassistant.step.child"); v.Str() != "script.x/c1" {
		t.Errorf("child link %q", v.Str())
	}
}

func TestStepName(t *testing.T) {
	var cfg map[string]any
	_ = json.Unmarshal([]byte(`{"actions": [{"choose": [{"alias": "Sol forte", "conditions": [
		{"condition": "numeric_state", "entity_id": "sensor.solar"},
		{"condition": "state", "entity_id": ["switch.a", "switch.b"]}]}]}]}`), &cfg)
	for path, want := range map[string]string{
		"action/0/choose/0":                          "choose 0: Sol forte",
		"action/0/choose/0/conditions/0":             "conditions 0: numeric_state",
		"action/0/choose/0/conditions/0/entity_id/0": "entity_id 0: sensor.solar",
		"action/0/choose/0/conditions/1/entity_id/1": "entity_id 1: switch.b",
		"action/0/choose/9":                          "choose 9",
		"action/0":                                   "action 0: choose",
	} {
		if got := stepName(path, cfg); got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
	if got := stepName("action/0/choose/1/sequence/0", nil); got != "sequence 0" {
		t.Errorf("no config: got %q", got)
	}
}

func TestTraceIDFallback(t *testing.T) {
	if traceIDFor("not-a-ulid", "r1") == traceIDFor("not-a-ulid", "r2") {
		t.Fatal("fallback trace ids must differ per run")
	}
}

// fakeHA speaks enough of the WebSocket API: auth, trace/list, trace/get.
type runList struct {
	mu  sync.Mutex
	ids []string
}

func (r *runList) add(id string) { r.mu.Lock(); r.ids = append(r.ids, id); r.mu.Unlock() }

func (r *runList) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ids...)
}

func fakeHA(t *testing.T, runs *runList) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		_ = wsjson.Write(ctx, c, map[string]string{"type": "auth_required"})
		var auth map[string]string
		if wsjson.Read(ctx, c, &auth) != nil || auth["access_token"] != "secret" {
			_ = wsjson.Write(ctx, c, map[string]string{"type": "auth_invalid"})
			return
		}
		_ = wsjson.Write(ctx, c, map[string]string{"type": "auth_ok"})
		for {
			var cmd map[string]any
			if wsjson.Read(ctx, c, &cmd) != nil {
				return
			}
			var result any = []any{}
			switch cmd["type"] {
			case "trace/list":
				if cmd["domain"] == "automation" {
					var l []traceSummary
					for _, id := range runs.all() {
						l = append(l, traceSummary{Domain: "automation", ItemID: "1791864237116", RunID: id, State: "stopped"})
					}
					result = l
				}
			case "get_states":
				result = json.RawMessage(`[
					{"entity_id": "sensor.sala_temp", "state": "21.5", "last_updated": "2026-11-25T10:00:00+00:00", "attributes": {"device_class": "temperature", "unit_of_measurement": "°C", "friendly_name": "Sala"}},
					{"entity_id": "switch.bomba", "state": "on", "attributes": {}},
					{"entity_id": "sensor.texto", "state": "abc", "attributes": {}},
					{"entity_id": "sensor.morto", "state": "unavailable", "attributes": {"device_class": "power"}},
					{"entity_id": "sensor.ignorado", "state": "1", "attributes": {}}]`)
			case "config/entity_registry/list":
				result = json.RawMessage(`[{"entity_id": "sensor.sala_temp", "device_id": "d1", "platform": "zha"}]`)
			case "config/device_registry/list":
				result = json.RawMessage(`[{"id": "d1", "name": "Termometro", "manufacturer": "Aqara", "model": "T1", "area_id": "sala"}]`)
			case "config/area_registry/list":
				result = json.RawMessage(`[{"area_id": "sala", "name": "Sala", "floor_id": "r0"}]`)
			case "config/floor_registry/list":
				result = json.RawMessage(`[{"floor_id": "r0", "name": "Rés-do-chão"}]`)
			case "trace/get":
				result = json.RawMessage(strings.Replace(sampleTrace, `"run_id": "r2"`, `"run_id": "`+cmd["run_id"].(string)+`"`, 1))
			}
			_ = wsjson.Write(ctx, c, map[string]any{"id": cmd["id"], "type": "result", "success": true, "result": result})
			if cmd["type"] == "subscribe_events" {
				for _, ev := range []string{
					`{"event_type": "state_changed", "data": {}, "time_fired": "2026-11-25T10:00:00+00:00", "context": {"id": "01KAV3RQ5Y2G3H4J5K6M7N8P9Q"}}`,
					`{"event_type": "call_service", "origin": "LOCAL", "time_fired": "2026-11-25T10:00:01+00:00",
					  "data": {"domain": "switch", "service": "turn_on", "service_data": {"entity_id": "switch.bomba"}},
					  "context": {"id": "01KAV3RQ5Y2G3H4J5K6M7N8P9Q", "parent_id": null, "user_id": "u1"}}`,
				} {
					_ = wsjson.Write(ctx, c, map[string]any{"id": cmd["id"], "type": "event", "event": json.RawMessage(ev)})
				}
			}
		}
	}))
}

func TestReceiverEmitsOnlyNewRuns(t *testing.T) {
	runs := &runList{ids: []string{"old1"}}
	srv := fakeHA(t, runs)
	defer srv.Close()

	cfg := createDefaultConfig().(*Config)
	cfg.Endpoint = "ws" + strings.TrimPrefix(srv.URL, "http")
	cfg.Token = "secret"
	cfg.PollInterval = 20 * time.Millisecond
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	sink := new(consumertest.TracesSink)
	r, err := NewFactory().CreateTraces(context.Background(), receivertest.NewNopSettings(componentType), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Shutdown(context.Background()) }()

	time.Sleep(100 * time.Millisecond) // primed: old1 must not be replayed
	if n := sink.SpanCount(); n != 0 {
		t.Fatalf("replayed runs stored before start: %d spans", n)
	}
	runs.add("new1")
	deadline := time.Now().Add(2 * time.Second)
	for sink.SpanCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := len(sink.AllTraces()); n != 1 {
		t.Fatalf("want exactly the new run, got %d traces", n)
	}
}

func TestConfigValidate(t *testing.T) {
	if err := (&Config{}).Validate(); err == nil {
		t.Fatal("empty config must be invalid")
	}
}

// A run still in the store must not be re-exported however old it is; a run that left
// the store is forgotten.
func TestPollKeepsListedRunsForgetsGoneOnes(t *testing.T) {
	runs := &runList{ids: []string{"old1"}}
	srv := fakeHA(t, runs)
	defer srv.Close()
	ctx := context.Background()
	c, err := dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	sink := new(consumertest.TracesSink)
	cfg := createDefaultConfig().(*Config)
	r := newReceiver(receivertest.NewNopSettings(componentType), cfg, sink)
	r.primed = true
	r.seen["automation/1791864237116/old1"] = time.Now().Add(-48 * time.Hour)
	r.seen["automation/1791864237116/gone"] = time.Now().Add(-time.Minute)
	if err := r.poll(ctx, c); err != nil {
		t.Fatal(err)
	}
	if n := sink.SpanCount(); n != 0 {
		t.Fatalf("re-exported a run still in the store: %d spans", n)
	}
	if _, ok := r.seen["automation/1791864237116/gone"]; ok {
		t.Fatal("kept a run Home Assistant no longer lists")
	}
	if _, ok := r.seen["automation/1791864237116/old1"]; !ok {
		t.Fatal("forgot a listed run")
	}
}

func TestStatsReceiver(t *testing.T) {
	sup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		stats := `{"data": {"cpu_percent": 150, "memory_usage": 1000, "network_rx": 1, "network_tx": 2, "blk_read": 3, "blk_write": 4}}`
		switch r.URL.Path {
		case "/addons":
			_, _ = w.Write([]byte(`{"data": {"addons": [{"slug": "core_ssh", "state": "started"}, {"slug": "off", "state": "stopped"}]}}`))
		case "/core/stats", "/supervisor/stats", "/addons/core_ssh/stats":
			_, _ = w.Write([]byte(stats))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer sup.Close()
	cfg := createDefaultConfig().(*Config)
	cfg.SupervisorEndpoint, cfg.Token, cfg.Containers = sup.URL, "secret", true
	sink := new(consumertest.MetricsSink)
	r := newStatsReceiver(receivertest.NewNopSettings(componentType), cfg, sink)
	md, err := r.scrape(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ms := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	if ms.At(0).Name() != "container.cpu.usage" || ms.At(0).Gauge().DataPoints().Len() != 3 {
		t.Fatalf("want cpu for core, supervisor and the started add-on, got %d", ms.At(0).Gauge().DataPoints().Len())
	}
	dp := ms.At(0).Gauge().DataPoints().At(2)
	if v, _ := dp.Attributes().Get("container.name"); v.Str() != "addon_core_ssh" || dp.DoubleValue() != 1.5 {
		t.Errorf("add-on cpu point: %s %v", v.Str(), dp.DoubleValue())
	}
	if n := ms.At(3).Sum().DataPoints().Len(); ms.At(3).Name() != "container.network.io" || n != 6 {
		t.Errorf("network.io points: %d", n)
	}
}

func TestEntityMetrics(t *testing.T) {
	srv := fakeHA(t, &runList{})
	defer srv.Close()
	cfg := createDefaultConfig().(*Config)
	cfg.Endpoint, cfg.Token, cfg.Entities = "ws"+strings.TrimPrefix(srv.URL, "http"), "secret", true
	cfg.ExcludeEntities = []string{"sensor.ignorado*"}
	r := newStatsReceiver(receivertest.NewNopSettings(componentType), cfg, new(consumertest.MetricsSink))
	md, err := r.scrape(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]pmetric.Metric{}
	ms := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	for i := 0; i < ms.Len(); i++ {
		got[ms.At(i).Name()] = ms.At(i)
	}
	temp, ok := got["homeassistant.sensor.temperature"]
	if !ok || temp.Unit() != "°C" || temp.Gauge().DataPoints().At(0).DoubleValue() != 21.5 {
		t.Fatalf("temperature metric missing or wrong: %v", got)
	}
	a := temp.Gauge().DataPoints().At(0).Attributes()
	for k, want := range map[string]string{"homeassistant.area.name": "Sala", "homeassistant.floor.name": "Rés-do-chão",
		"homeassistant.device.manufacturer": "Aqara", "homeassistant.entity.platform": "zha"} {
		if v, _ := a.Get(k); v.Str() != want {
			t.Errorf("%s = %q, want %q", k, v.Str(), want)
		}
	}
	if v := got["homeassistant.switch.on"].Gauge().DataPoints().At(0).DoubleValue(); v != 1 {
		t.Errorf("switch on = %v", v)
	}
	avail := got["homeassistant.entity.available"].Gauge().DataPoints()
	if avail.Len() != 4 { // the excluded entity is left out
		t.Errorf("available points: %d", avail.Len())
	}
	if _, ok := got["homeassistant.sensor.power"]; ok {
		t.Error("unavailable sensor must not produce a value")
	}
	if v := got["homeassistant.core.up"].Gauge().DataPoints().At(0).DoubleValue(); v != 1 {
		t.Errorf("core.up = %v", v)
	}
	if v := got["homeassistant.entity.last_updated"].Gauge().DataPoints().At(0).DoubleValue(); v != 1795600800 {
		t.Errorf("last_updated = %v", v)
	}

	// Core down: the scrape still reports core.up = 0.
	srv.Close()
	md, err = r.scrape(context.Background())
	if err == nil {
		t.Fatal("want an error with Core down")
	}
	m := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0)
	if m.Name() != "homeassistant.core.up" || m.Gauge().DataPoints().At(0).DoubleValue() != 0 {
		t.Errorf("Core down: %s = %v", m.Name(), m.Gauge().DataPoints().At(0).DoubleValue())
	}
}

func TestEventsReceiver(t *testing.T) {
	srv := fakeHA(t, &runList{})
	defer srv.Close()
	cfg := createDefaultConfig().(*Config)
	cfg.Endpoint, cfg.Token = "ws"+strings.TrimPrefix(srv.URL, "http"), "secret"
	sink := new(consumertest.LogsSink)
	r := newEventsReceiver(receivertest.NewNopSettings(componentType), cfg, sink)
	if err := r.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Shutdown(context.Background()) }()
	deadline := time.Now().Add(2 * time.Second)
	for sink.LogRecordCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := sink.LogRecordCount(); n != 1 { // state_changed is excluded by default
		t.Fatalf("want 1 record, got %d", n)
	}
	lr := sink.AllLogs()[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	if lr.Body().Type() != pcommon.ValueTypeEmpty {
		t.Error("events carry no body")
	}
	if lr.EventName() != "homeassistant.call_service" || lr.TraceID() != traceIDFor("01KAV3RQ5Y2G3H4J5K6M7N8P9Q", "") {
		t.Errorf("event %q trace %s: want call_service in the run's trace", lr.EventName(), lr.TraceID())
	}
	if v, _ := lr.Attributes().Get("homeassistant.event.data.service_data"); v.Str() != `{"entity_id":"switch.bomba"}` {
		t.Errorf("service_data = %q", v.Str())
	}
	if v, _ := lr.Attributes().Get("user.id"); v.Str() != "u1" {
		t.Errorf("user.id = %q", v.Str())
	}
}
