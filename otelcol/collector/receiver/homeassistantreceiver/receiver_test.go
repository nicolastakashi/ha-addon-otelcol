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
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

// A real trace/get payload shape (trimmed) from Home Assistant.
const sampleTrace = `{
  "domain": "automation", "item_id": "1791864237116", "run_id": "r2",
  "state": "stopped", "script_execution": "finished",
  "trigger": "time pattern",
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
    "action/1/if": [{"path": "action/1/if", "timestamp": "2026-11-24T21:05:00.190000+00:00"}],
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
		{"action 1", 0, 300},
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
			case "trace/get":
				result = json.RawMessage(strings.Replace(sampleTrace, `"run_id": "r2"`, `"run_id": "`+cmd["run_id"].(string)+`"`, 1))
			}
			_ = wsjson.Write(ctx, c, map[string]any{"id": cmd["id"], "type": "result", "success": true, "result": result})
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
