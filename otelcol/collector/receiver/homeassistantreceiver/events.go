package homeassistantreceiver

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/coder/websocket/wsjson"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
)

type haEvent struct {
	EventType string         `json:"event_type"`
	Data      map[string]any `json:"data"`
	Origin    string         `json:"origin"`
	TimeFired time.Time      `json:"time_fired"`
	Context   struct {
		ID       string `json:"id"`
		ParentID string `json:"parent_id"`
		UserID   string `json:"user_id"`
	} `json:"context"`
}

// eventsReceiver is the logs side of the receiver: every Home Assistant bus event (except
// exclude_event_types) becomes a log record. Its trace id is derived from the event's
// context id exactly like a run's trace id, so the events an automation causes
// (automation_triggered, call_service, the state changes) land in the run's trace.
type eventsReceiver struct {
	cfg    *Config
	logger *zap.Logger
	next   consumer.Logs

	cancel context.CancelFunc
	done   chan struct{}
}

func newEventsReceiver(set receiver.Settings, cfg *Config, next consumer.Logs) *eventsReceiver {
	return &eventsReceiver{cfg: cfg, logger: set.Logger, next: next}
}

func (r *eventsReceiver) Start(_ context.Context, _ component.Host) error {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel, r.done = cancel, make(chan struct{})
	go func() {
		defer close(r.done)
		backoff := time.Second
		for ctx.Err() == nil {
			err := r.listen(ctx)
			if ctx.Err() != nil {
				return
			}
			r.logger.Warn("Home Assistant event stream failed, reconnecting", zap.Error(err), zap.Duration("in", backoff))
			if !sleep(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, time.Minute)
		}
	}()
	return nil
}

func (r *eventsReceiver) Shutdown(context.Context) error {
	if r.cancel != nil {
		r.cancel()
		<-r.done
	}
	return nil
}

func (r *eventsReceiver) listen(ctx context.Context) error {
	c, err := dial(ctx, r.cfg.Endpoint, string(r.cfg.Token))
	if err != nil {
		return err
	}
	defer c.close()
	if err := c.call(ctx, map[string]any{"type": "subscribe_events"}, new(any)); err != nil {
		return err
	}
	for {
		var msg struct {
			Type  string          `json:"type"`
			Event json.RawMessage `json:"event"`
		}
		if err := wsjson.Read(ctx, c.conn, &msg); err != nil {
			return err
		}
		if msg.Type != "event" {
			continue
		}
		var ev haEvent
		if err := json.Unmarshal(msg.Event, &ev); err != nil {
			return fmt.Errorf("decode event: %w", err)
		}
		if slices.Contains(r.cfg.ExcludeEventTypes, ev.EventType) {
			continue
		}
		if err := r.next.ConsumeLogs(ctx, toLogs(ev)); err != nil {
			r.logger.Warn("Dropping Home Assistant event", zap.String("event_type", ev.EventType), zap.Error(err))
		}
	}
}

func toLogs(ev haEvent) plog.Logs {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("container.name", "homeassistant") // events come from Core
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName(scopeName)
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(ev.TimeFired))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	// OTel Events: a namespaced event name per event type, payload in attributes, no body.
	lr.SetEventName("homeassistant." + ev.EventType)
	lr.SetSeverityNumber(plog.SeverityNumberInfo)
	lr.SetSeverityText("INFO")
	if ev.Context.ID != "" {
		lr.SetTraceID(traceIDFor(ev.Context.ID, ev.Context.ID))
	}
	a := lr.Attributes()
	a.PutStr(AttrHomeassistantEventType, ev.EventType)
	putIf(a, AttrHomeassistantEventOrigin, ev.Origin)
	putIf(a, AttrHomeassistantContextId, ev.Context.ID)
	putIf(a, AttrHomeassistantContextParentId, ev.Context.ParentID)
	putIf(a, "user.id", ev.Context.UserID)
	for k, v := range ev.Data {
		key := AttrHomeassistantEventData + "." + k
		switch v := v.(type) {
		case string:
			a.PutStr(key, v)
		case bool:
			a.PutBool(key, v)
		case float64:
			a.PutDouble(key, v)
		case nil:
		default: // nested data (service_data, new_state...): keep it as JSON
			if b, err := json.Marshal(v); err == nil {
				a.PutStr(key, string(b))
			}
		}
	}
	return ld
}
