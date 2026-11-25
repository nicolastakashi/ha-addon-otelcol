package homeassistantreceiver

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
)

type haReceiver struct {
	cfg    *Config
	logger *zap.Logger
	next   consumer.Traces

	cancel context.CancelFunc
	done   chan struct{}

	seen   map[string]time.Time // domain/item/run -> last poll that listed it
	primed bool                 // runs already stored at startup are not replayed
}

func newReceiver(set receiver.Settings, cfg *Config, next consumer.Traces) *haReceiver {
	return &haReceiver{cfg: cfg, logger: set.Logger, next: next, seen: map[string]time.Time{}}
}

func (r *haReceiver) Start(_ context.Context, _ component.Host) error {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	go r.run(ctx)
	return nil
}

func (r *haReceiver) Shutdown(context.Context) error {
	if r.cancel != nil {
		r.cancel()
		<-r.done
	}
	return nil
}

func (r *haReceiver) run(ctx context.Context) {
	defer close(r.done)
	backoff := time.Second
	for ctx.Err() == nil {
		c, err := dial(ctx, r.cfg.Endpoint, string(r.cfg.Token))
		if err != nil {
			r.logger.Warn("Home Assistant connection failed, retrying", zap.Error(err), zap.Duration("in", backoff))
			if !sleep(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = time.Second
		r.logger.Info("Connected to Home Assistant", zap.String("endpoint", r.cfg.Endpoint))
		r.pollLoop(ctx, c)
		c.close()
	}
}

func (r *haReceiver) pollLoop(ctx context.Context, c *haClient) {
	t := time.NewTicker(r.cfg.PollInterval)
	defer t.Stop()
	for {
		if err := r.poll(ctx, c); err != nil {
			if ctx.Err() == nil {
				r.logger.Warn("Polling Home Assistant traces failed, reconnecting", zap.Error(err))
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ponytail: polling trace/list; a burst of more than stored_traces runs of one
// automation within a poll interval loses the oldest. Subscribe to
// automation_triggered/script_started and fetch eagerly if that matters.
func (r *haReceiver) poll(ctx context.Context, c *haClient) error {
	now := time.Now()
	for _, domain := range r.cfg.Domains {
		var list []traceSummary
		if err := c.call(ctx, map[string]any{"type": "trace/list", "domain": domain}, &list); err != nil {
			return err
		}
		for _, s := range list {
			key := s.Domain + "/" + s.ItemID + "/" + s.RunID
			if s.State != "stopped" {
				continue
			}
			if !r.seen[key].IsZero() {
				r.seen[key] = now // still in the store: keep it
				continue
			}
			if !r.primed {
				r.seen[key] = now
				continue
			}
			var d traceDetail
			err := c.call(ctx, map[string]any{"type": "trace/get", "domain": s.Domain, "item_id": s.ItemID, "run_id": s.RunID}, &d)
			if err != nil {
				return err
			}
			if err := r.next.ConsumeTraces(ctx, toTraces(d)); err != nil {
				r.logger.Warn("Dropping Home Assistant trace", zap.String("run", key), zap.Error(err))
			}
			r.seen[key] = now
		}
	}
	r.primed = true
	// Forget runs Home Assistant no longer lists; forgetting by age re-exported
	// runs of automations that run less than once per period.
	for k, at := range r.seen {
		if at.Before(now) {
			delete(r.seen, k)
		}
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
