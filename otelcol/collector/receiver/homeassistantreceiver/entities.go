package homeassistantreceiver

import (
	"context"
	"path"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

type haState struct {
	EntityID   string         `json:"entity_id"`
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes"`
}

type entityEntry struct {
	EntityID string `json:"entity_id"`
	DeviceID string `json:"device_id"`
	AreaID   string `json:"area_id"`
	Platform string `json:"platform"`
}

type deviceEntry struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	NameByUser   string `json:"name_by_user"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	AreaID       string `json:"area_id"`
}

type areaEntry struct {
	AreaID  string `json:"area_id"`
	Name    string `json:"name"`
	FloorID string `json:"floor_id"`
}

type floorEntry struct {
	FloorID string `json:"floor_id"`
	Name    string `json:"name"`
}

// numericDomains are domains whose numeric state is a measurement or a setting.
var numericDomains = map[string]bool{"sensor": true, "number": true, "input_number": true, "counter": true}

// entityMetrics reads every entity state plus the entity, device, area and floor
// registries over the WebSocket API and turns them into gauges:
//
//	homeassistant.sensor.<device_class>  numeric sensor state (unit from the entity)
//	homeassistant.<domain>.value         numeric state of other numeric entities
//	homeassistant.<domain>.on            1/0 for on/off entities
//	homeassistant.entity.available       0 when unavailable, else 1
func (r *statsReceiver) entityMetrics(ctx context.Context, ms pmetric.MetricSlice, now pcommon.Timestamp) error {
	c, err := dial(ctx, r.cfg.Endpoint, string(r.cfg.Token))
	if err != nil {
		return err
	}
	defer c.close()
	var (
		states  []haState
		ents    []entityEntry
		devices []deviceEntry
		areas   []areaEntry
		floors  []floorEntry
	)
	if err := c.call(ctx, map[string]any{"type": "get_states"}, &states); err != nil {
		return err
	}
	// Registries only add context; a failure (old Core, missing floors) is not fatal.
	_ = c.call(ctx, map[string]any{"type": "config/entity_registry/list"}, &ents)
	_ = c.call(ctx, map[string]any{"type": "config/device_registry/list"}, &devices)
	_ = c.call(ctx, map[string]any{"type": "config/area_registry/list"}, &areas)
	_ = c.call(ctx, map[string]any{"type": "config/floor_registry/list"}, &floors)

	entByID := map[string]entityEntry{}
	for _, e := range ents {
		entByID[e.EntityID] = e
	}
	devByID := map[string]deviceEntry{}
	for _, d := range devices {
		devByID[d.ID] = d
	}
	areaByID := map[string]areaEntry{}
	for _, a := range areas {
		areaByID[a.AreaID] = a
	}
	floorByID := map[string]string{}
	for _, f := range floors {
		floorByID[f.FloorID] = f.Name
	}

	metrics := map[string]pmetric.Metric{} // name + unit -> metric
	get := func(name, unit string) pmetric.Metric {
		if m, ok := metrics[name+"\x00"+unit]; ok {
			return m
		}
		m := gauge(ms, name, unit, "")
		metrics[name+"\x00"+unit] = m
		return m
	}

	for _, s := range states {
		if r.excluded(s.EntityID) {
			continue
		}
		domain, _, _ := strings.Cut(s.EntityID, ".")
		attrs := pcommon.NewMap()
		attrs.PutStr("homeassistant.entity.id", s.EntityID)
		attrs.PutStr("homeassistant.entity.domain", domain)
		putAttr(attrs, "homeassistant.entity.name", s.Attributes["friendly_name"])
		putAttr(attrs, "homeassistant.entity.device_class", s.Attributes["device_class"])
		e := entByID[s.EntityID]
		putAttr(attrs, "homeassistant.entity.platform", e.Platform)
		areaID := e.AreaID
		if d, ok := devByID[e.DeviceID]; ok {
			name := d.NameByUser
			if name == "" {
				name = d.Name
			}
			putAttr(attrs, "homeassistant.device.name", name)
			putAttr(attrs, "homeassistant.device.manufacturer", d.Manufacturer)
			putAttr(attrs, "homeassistant.device.model", d.Model)
			if areaID == "" {
				areaID = d.AreaID
			}
		}
		if a, ok := areaByID[areaID]; ok {
			putAttr(attrs, "homeassistant.area.name", a.Name)
			putAttr(attrs, "homeassistant.floor.name", floorByID[a.FloorID])
		}

		available := 1.0
		if s.State == "unavailable" {
			available = 0
		}
		add(get("homeassistant.entity.available", "1"), now, attrs, available)

		switch {
		case s.State == "on" || s.State == "off":
			v := 0.0
			if s.State == "on" {
				v = 1
			}
			add(get("homeassistant."+domain+".on", "1"), now, attrs, v)
		case numericDomains[domain]:
			v, err := strconv.ParseFloat(s.State, 64)
			if err != nil {
				continue // unknown, unavailable, or a text sensor
			}
			unit, _ := s.Attributes["unit_of_measurement"].(string)
			name := "homeassistant." + domain + ".value"
			if dc, _ := s.Attributes["device_class"].(string); domain == "sensor" && dc != "" {
				name = "homeassistant.sensor." + dc
			}
			add(get(name, unit), now, attrs, v)
		}
	}
	return nil
}

func (r *statsReceiver) excluded(entityID string) bool {
	for _, p := range r.cfg.ExcludeEntities {
		if ok, _ := path.Match(p, entityID); ok {
			return true
		}
	}
	return false
}

func putAttr(m pcommon.Map, k string, v any) {
	if s, ok := v.(string); ok && s != "" {
		m.PutStr(k, s)
	}
}

func add(m pmetric.Metric, now pcommon.Timestamp, attrs pcommon.Map, v float64) {
	dp := m.Gauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(now)
	attrs.CopyTo(dp.Attributes())
	dp.SetDoubleValue(v)
}
